package csapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

type fakeSchemaObjectStore struct {
	puts        map[string][]byte
	putCalls    []string
	getCalls    []string
	getSequence [][]byte
	putErr      error
	getErr      error
}

func (f *fakeSchemaObjectStore) PutBytes(_ context.Context, name string, data []byte) (*jetstream.ObjectInfo, error) {
	f.putCalls = append(f.putCalls, name)
	if f.putErr != nil {
		return nil, f.putErr
	}
	if f.puts == nil {
		f.puts = make(map[string][]byte)
	}
	f.puts[name] = append([]byte(nil), data...)
	return &jetstream.ObjectInfo{
		ObjectMeta: jetstream.ObjectMeta{Name: name},
		Size:       uint64(len(data)),
	}, nil
}

func (f *fakeSchemaObjectStore) GetBytes(_ context.Context, name string, _ ...jetstream.GetObjectOpt) ([]byte, error) {
	f.getCalls = append(f.getCalls, name)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if len(f.getSequence) > 0 {
		index := len(f.getCalls) - 1
		if index >= len(f.getSequence) {
			index = len(f.getSequence) - 1
		}
		return append([]byte(nil), f.getSequence[index]...), nil
	}
	data, ok := f.puts[name]
	if !ok {
		return nil, jetstream.ErrObjectNotFound
	}
	return append([]byte(nil), data...), nil
}

func TestCreateSchemaArtifact_StoresBytesAndCreatesTypedEntity(t *testing.T) {
	fakeNATS := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 1),
	}
	c := newTestComponent(t, fakeNATS)
	store := &fakeSchemaObjectStore{}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)

	parentID := "c360.semconnect.systems.csapi.datastream.temp-feed"
	rel, err := c.createSchemaArtifact(
		context.Background(),
		parentID,
		csapivocab.HasResultSchema,
		json.RawMessage(testSWEDataRecordSchema),
		Identity{Subject: "alice"},
	)
	if err != nil {
		t.Fatalf("createSchemaArtifact: %v", err)
	}
	if rel.Subject != parentID {
		t.Errorf("relationship subject: got %q want %q", rel.Subject, parentID)
	}
	if rel.Predicate != csapivocab.HasResultSchema {
		t.Errorf("relationship predicate: got %q want %q", rel.Predicate, csapivocab.HasResultSchema)
	}
	if rel.Datatype != message.EntityReferenceDatatype {
		t.Errorf("relationship datatype: got %q want %q", rel.Datatype, message.EntityReferenceDatatype)
	}
	artifactID, ok := rel.Object.(string)
	if !ok || artifactID == "" {
		t.Fatalf("relationship object: got %#v want artifact entity ID string", rel.Object)
	}
	if !strings.HasPrefix(artifactID, c.cfg.SchemaArtifactIDPrefix+".sha256-") {
		t.Errorf("artifact ID is not content-addressed: %q", artifactID)
	}

	key := schemaArtifactObjectKey(artifactID)
	stored, ok := store.puts[key]
	if !ok {
		t.Fatalf("schema bytes not stored at key %q; puts=%+v", key, store.puts)
	}
	if !json.Valid(stored) {
		t.Fatalf("stored schema is not JSON: %s", stored)
	}
	wantCanonical, err := normalizeSWESchema(json.RawMessage(testSWEDataRecordSchema))
	if err != nil {
		t.Fatalf("normalize want schema: %v", err)
	}
	if !bytes.Equal(stored, wantCanonical) {
		t.Errorf("stored schema:\n got %s\nwant %s", stored, wantCanonical)
	}

	if fakeNATS.gotSubject != SubjectEntityCreate {
		t.Fatalf("mutation subject: got %q want %q", fakeNATS.gotSubject, SubjectEntityCreate)
	}
	var sent graph.CreateEntityRequest
	if err := json.Unmarshal(fakeNATS.gotBody, &sent); err != nil {
		t.Fatalf("decode mutation body: %v", err)
	}
	if sent.Entity == nil {
		t.Fatal("mutation body missing entity")
	}
	if sent.Entity.ID != artifactID {
		t.Errorf("entity ID: got %q want %q", sent.Entity.ID, artifactID)
	}
	if sent.Entity.StorageRef == nil {
		t.Fatal("entity missing StorageRef")
	}
	if sent.Entity.StorageRef.StorageInstance != schemaArtifactStorageInstance {
		t.Errorf("storage instance: got %q want %q", sent.Entity.StorageRef.StorageInstance, schemaArtifactStorageInstance)
	}
	if sent.Entity.StorageRef.Key != key {
		t.Errorf("storage key: got %q want %q", sent.Entity.StorageRef.Key, key)
	}
	if sent.Entity.StorageRef.ContentType != schemaArtifactContentType {
		t.Errorf("content type: got %q want %q", sent.Entity.StorageRef.ContentType, schemaArtifactContentType)
	}
	if sent.Entity.StorageRef.Size != int64(len(stored)) {
		t.Errorf("storage size: got %d want %d", sent.Entity.StorageRef.Size, len(stored))
	}
	if len(sent.Triples) != 1 {
		t.Fatalf("artifact triples: got %+v want exactly one type triple", sent.Triples)
	}
	if tr := sent.Triples[0]; tr.Subject != artifactID || tr.Predicate != sensorml.PredType || tr.Object != csapivocab.SWESchemaDocument {
		t.Errorf("artifact type triple: got %+v", tr)
	}
}

func TestCreateSchemaArtifact_RequiresInitializedStore(t *testing.T) {
	fakeNATS := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 1)}
	c := newTestComponent(t, fakeNATS)

	_, err := c.createSchemaArtifact(
		context.Background(),
		"c360.semconnect.systems.csapi.datastream.temp-feed",
		csapivocab.HasResultSchema,
		json.RawMessage(testSWEDataRecordSchema),
		Identity{},
	)
	if err == nil {
		t.Fatal("createSchemaArtifact: got nil error, want transient store error")
	}
	if !errs.IsTransient(err) {
		t.Fatalf("error class: got %T %[1]v want transient", err)
	}
	if fakeNATS.gotSubject != "" {
		t.Fatalf("graph mutation should not happen without store; got %q", fakeNATS.gotSubject)
	}
}

func TestCreateSchemaArtifact_ConflictVerifiesExactImmutableArtifact(t *testing.T) {
	canonical, err := normalizeSWESchema(json.RawMessage(testSWEDataRecordSchema))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	digest := sha256.Sum256(canonical)
	artifactID := fmt.Sprintf("%s.sha256-%x", cfg.SchemaArtifactIDPrefix, digest)
	storageRef := &message.StorageReference{
		StorageInstance: schemaArtifactStorageInstance,
		Key:             schemaArtifactObjectKey(artifactID),
		ContentType:     schemaArtifactContentType,
		Size:            int64(len(canonical)),
	}
	existing := graph.EntityState{
		ID: artifactID,
		Triples: []message.Triple{{
			Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument,
		}},
		StorageRef: storageRef,
	}
	conflictBody, conflictHeader := encodeEntityMutationFailure(t, graph.ErrorCodeEntityExists, "exists")
	fakeNATS := &crdFakeRequester{
		entityReply: mustMarshal(t, existing), batchReply: conflictBody, batchHeader: conflictHeader,
	}
	c := newComponentWithRequester(t, fakeNATS)
	store := &fakeSchemaObjectStore{puts: map[string][]byte{storageRef.Key: canonical}}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)

	rel, err := c.createSchemaArtifact(context.Background(),
		"c360.semconnect.systems.csapi.datastream.temp-feed",
		csapivocab.HasResultSchema, json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err != nil {
		t.Fatalf("identical immutable conflict: %v", err)
	}
	if rel.Object != artifactID || fakeNATS.batchCount != 1 || fakeNATS.entityQueryCalls != 1 {
		t.Fatalf("immutable conflict result=%+v create=%d exact=%d", rel, fakeNATS.batchCount, fakeNATS.entityQueryCalls)
	}
	if len(store.putCalls) != 0 {
		t.Fatalf("immutable conflict must not overwrite object; puts=%v", store.putCalls)
	}
	if len(store.getCalls) != 2 {
		t.Fatalf("immutable conflict must verify bytes before and after exact graph verification; gets=%v", store.getCalls)
	}
	var create graph.CreateEntityRequest
	if err := json.Unmarshal(fakeNATS.batchBody, &create); err != nil || create.Entity == nil {
		t.Fatalf("conflict attempt was not strict create: err=%v request=%+v", err, create)
	}
}

func TestCreateSchemaArtifact_ConflictRejectsIntegrityMismatch(t *testing.T) {
	canonical, err := normalizeSWESchema(json.RawMessage(testSWEDataRecordSchema))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	digest := sha256.Sum256(canonical)
	artifactID := fmt.Sprintf("%s.sha256-%x", cfg.SchemaArtifactIDPrefix, digest)
	existing := graph.EntityState{
		ID: artifactID,
		Triples: []message.Triple{{
			Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument,
		}},
		StorageRef: &message.StorageReference{
			StorageInstance: "wrong-provider", Key: schemaArtifactObjectKey(artifactID),
			ContentType: schemaArtifactContentType, Size: int64(len(canonical)),
		},
	}
	conflictBody, conflictHeader := encodeEntityMutationFailure(t, graph.ErrorCodeEntityExists, "exists")
	fakeNATS := &crdFakeRequester{
		entityReply: mustMarshal(t, existing), batchReply: conflictBody, batchHeader: conflictHeader,
	}
	c := newComponentWithRequester(t, fakeNATS)
	store := &fakeSchemaObjectStore{puts: map[string][]byte{schemaArtifactObjectKey(artifactID): canonical}}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)

	_, err = c.createSchemaArtifact(context.Background(),
		"c360.semconnect.systems.csapi.datastream.temp-feed",
		csapivocab.HasResultSchema, json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err == nil || errors.Is(err, errEntityConflict) || errs.IsTransient(err) || errs.IsInvalid(err) {
		t.Fatalf("integrity mismatch classification = %#v", err)
	}
	if fakeNATS.batchCount != 1 || fakeNATS.entityQueryCalls != 1 {
		t.Fatalf("integrity mismatch calls: create=%d exact=%d", fakeNATS.batchCount, fakeNATS.entityQueryCalls)
	}
	if len(store.putCalls) != 0 {
		t.Fatalf("metadata conflict must not overwrite object; puts=%v", store.putCalls)
	}
}

func TestCreateSchemaArtifact_ConflictRejectsObjectByteMismatchWithoutOverwrite(t *testing.T) {
	canonical, err := normalizeSWESchema(json.RawMessage(testSWEDataRecordSchema))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	digest := sha256.Sum256(canonical)
	artifactID := fmt.Sprintf("%s.sha256-%x", cfg.SchemaArtifactIDPrefix, digest)
	key := schemaArtifactObjectKey(artifactID)
	storageRef := &message.StorageReference{
		StorageInstance: schemaArtifactStorageInstance,
		Key:             key, ContentType: schemaArtifactContentType, Size: int64(len(canonical)),
	}
	existing := graph.EntityState{
		ID: artifactID,
		Triples: []message.Triple{{
			Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument,
		}},
		StorageRef: storageRef,
	}
	conflictBody, conflictHeader := encodeEntityMutationFailure(t, graph.ErrorCodeEntityExists, "exists")
	fakeNATS := &crdFakeRequester{
		entityReply: mustMarshal(t, existing), batchReply: conflictBody, batchHeader: conflictHeader,
	}
	c := newComponentWithRequester(t, fakeNATS)
	wrongBytes := []byte(`{"type":"DataRecord","fields":[]}`)
	store := &fakeSchemaObjectStore{
		puts:        map[string][]byte{key: canonical},
		getSequence: [][]byte{canonical, wrongBytes},
	}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)

	_, err = c.createSchemaArtifact(context.Background(),
		"c360.semconnect.systems.csapi.datastream.temp-feed",
		csapivocab.HasResultSchema, json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err == nil || errs.IsInvalid(err) || errs.IsTransient(err) || errors.Is(err, errEntityConflict) {
		t.Fatalf("object mismatch classification = %#v", err)
	}
	if len(store.putCalls) != 0 {
		t.Fatalf("object mismatch must not overwrite bytes; puts=%v", store.putCalls)
	}
	if fakeNATS.batchCount != 1 || fakeNATS.entityQueryCalls != 1 || len(store.getCalls) != 2 {
		t.Fatalf("conflict verification calls: create=%d exact=%d gets=%v",
			fakeNATS.batchCount, fakeNATS.entityQueryCalls, store.getCalls)
	}
	if !bytes.Equal(store.puts[key], canonical) {
		t.Fatalf("conflict verification overwrote object: got %q want %q", store.puts[key], canonical)
	}
}

func TestCreateSchemaArtifact_PreexistingMismatchSkipsGraphAndOverwrite(t *testing.T) {
	canonical, err := normalizeSWESchema(json.RawMessage(testSWEDataRecordSchema))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	digest := sha256.Sum256(canonical)
	artifactID := fmt.Sprintf("%s.sha256-%x", cfg.SchemaArtifactIDPrefix, digest)
	key := schemaArtifactObjectKey(artifactID)
	fakeNATS := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 1)}
	c := newTestComponent(t, fakeNATS)
	wrongBytes := []byte(`{"type":"DataRecord","fields":[]}`)
	store := &fakeSchemaObjectStore{puts: map[string][]byte{key: wrongBytes}}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)

	_, err = c.createSchemaArtifact(context.Background(),
		"c360.semconnect.systems.csapi.datastream.temp-feed",
		csapivocab.HasResultSchema, json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err == nil || errs.IsInvalid(err) || errs.IsTransient(err) {
		t.Fatalf("preexisting object mismatch classification = %#v", err)
	}
	if len(store.putCalls) != 0 {
		t.Fatalf("preexisting mismatch must not overwrite bytes; puts=%v", store.putCalls)
	}
	if fakeNATS.gotSubject != "" {
		t.Fatalf("graph mutation occurred for mismatched object: %q", fakeNATS.gotSubject)
	}
	if !bytes.Equal(store.puts[key], wrongBytes) {
		t.Fatalf("preexisting object bytes changed: got %q want %q", store.puts[key], wrongBytes)
	}
}

func TestCreateSchemaArtifact_PutFailureSkipsGraphAndRetryHeals(t *testing.T) {
	fakeNATS := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 1)}
	c := newTestComponent(t, fakeNATS)
	store := &fakeSchemaObjectStore{putErr: errors.New("object store write failed")}
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)
	parentID := "c360.semconnect.systems.csapi.datastream.temp-feed"

	_, err := c.createSchemaArtifact(context.Background(), parentID, csapivocab.HasResultSchema,
		json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err == nil {
		t.Fatal("PutBytes failure was accepted")
	}
	if fakeNATS.gotSubject != "" {
		t.Fatalf("graph mutation occurred after failed PutBytes: %q", fakeNATS.gotSubject)
	}
	if len(store.puts) != 0 {
		t.Fatalf("failed PutBytes left object bytes: %+v", store.puts)
	}

	store.putErr = nil
	rel, err := c.createSchemaArtifact(context.Background(), parentID, csapivocab.HasResultSchema,
		json.RawMessage(testSWEDataRecordSchema), Identity{})
	if err != nil {
		t.Fatalf("retry did not heal artifact creation: %v", err)
	}
	artifactID, ok := rel.Object.(string)
	if !ok || artifactID == "" {
		t.Fatalf("retry relationship = %+v", rel)
	}
	key := schemaArtifactObjectKey(artifactID)
	if len(store.puts[key]) == 0 || fakeNATS.gotSubject != SubjectEntityCreate {
		t.Fatalf("retry did not store then create: object=%q subject=%q", store.puts[key], fakeNATS.gotSubject)
	}
}

func TestConfigValidateSchemaArtifactSettings(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config: %v", err)
	}

	cfg.SchemaArtifactsBucket = "bad.bucket"
	if err := cfg.Validate(); err == nil {
		t.Fatal("bucket with dot: got nil error, want validation failure")
	}

	cfg = DefaultConfig()
	cfg.SchemaArtifactsBucket = "OTHER_VALID_BUCKET"
	if err := cfg.Validate(); err == nil {
		t.Fatal("valid but noncanonical bucket: got nil error, want validation failure")
	}

	cfg = DefaultConfig()
	cfg.SchemaArtifactIDPrefix = "too.short.prefix"
	if err := cfg.Validate(); err == nil {
		t.Fatal("bad schema artifact prefix: got nil error, want validation failure")
	}
}
