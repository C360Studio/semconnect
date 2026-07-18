package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	puts map[string][]byte
	err  error
}

func (f *fakeSchemaObjectStore) PutBytes(_ context.Context, name string, data []byte) (*jetstream.ObjectInfo, error) {
	if f.err != nil {
		return nil, f.err
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
	data, ok := f.puts[name]
	if !ok {
		return nil, errors.New("not found")
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
	if got, want := artifactID, c.cfg.SchemaArtifactIDPrefix+"."+uniqueIDToToken(parentID+"-resultSchema"); got != want {
		t.Errorf("artifact ID: got %q want %q", got, want)
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

	if fakeNATS.gotSubject != SubjectEntityCreateWithTriples {
		t.Fatalf("mutation subject: got %q want %q", fakeNATS.gotSubject, SubjectEntityCreateWithTriples)
	}
	var sent graph.CreateEntityWithTriplesRequest
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
	if sent.Entity.StorageRef.StorageInstance != c.cfg.SchemaArtifactsBucket {
		t.Errorf("storage instance: got %q want %q", sent.Entity.StorageRef.StorageInstance, c.cfg.SchemaArtifactsBucket)
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
	cfg.SchemaArtifactIDPrefix = "too.short.prefix"
	if err := cfg.Validate(); err == nil {
		t.Fatal("bad schema artifact prefix: got nil error, want validation failure")
	}
}
