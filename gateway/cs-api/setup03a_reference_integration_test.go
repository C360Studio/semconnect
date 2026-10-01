//go:build integration

package csapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func setup03AGraph(t *testing.T) (*Component, *natsclient.Client) {
	t.Helper()
	server := startEmbeddedNATSServer(t, true)
	client := connectSemStreamsClient(t, server.ClientURL())
	c, err := New(DefaultConfig(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.bindProjectionContracts(t.Context()); err != nil {
		t.Fatal(err)
	}
	startBeta160GraphIngest(t, t.Context(), client)
	bridgeBeta160AuthorityQueries(t, t.Context(), client)
	return c, client
}

func TestSetup03AReferenceTypedMutations(t *testing.T) {
	t.Run("authority_lifecycle", func(t *testing.T) {
		c, _ := setup03AGraph(t)
		expected := setup03AExpected(t)
		id, triples, err := c.buildSystemTriplesFromSensorML(setup03AFixture(t, "system.json"))
		if err != nil {
			t.Fatal(err)
		}
		ctx, identity := t.Context(), Identity{Subject: "setup03a-reference"}
		if err := c.ingestProjectedTriples(ctx, id, triples, systemProjectionMessageType, identity); err != nil {
			t.Fatalf("create: %v", err)
		}
		created, err := c.fetchEntityExact(ctx, id)
		if err != nil || created.KVRevision == 0 {
			t.Fatalf("exact create read: %+v %v", created, err)
		}
		requireStoredTriple(t, created, sensorml.PredHosts, expected.AbsentTargetID)
		if _, err := c.fetchEntityExact(ctx, expected.AbsentTargetID); !errors.Is(err, errEntityNotFound) {
			t.Fatalf("relationship target must remain absent: %v", err)
		}
		if err := c.ingestProjectedTriples(ctx, id, triples, systemProjectionMessageType, identity); !errors.Is(err, errEntityConflict) {
			t.Fatalf("duplicate strict create: %v", err)
		}
		desired := append([]message.Triple(nil), triples...)
		for i := range desired {
			if desired[i].Predicate == sensorml.PredLabel {
				desired[i].Object = "SETUP 03A reconciled"
			}
		}
		if err := c.reconcileEntity(ctx, created, desired, identity, "referenceReconcile"); err != nil {
			t.Fatal(err)
		}
		reconciled, err := c.fetchEntityExact(ctx, id)
		if err != nil || reconciled.KVRevision <= created.KVRevision {
			t.Fatalf("exact reconcile revision: %+v %v", reconciled, err)
		}
		requireStoredTriple(t, reconciled, sensorml.PredLabel, "SETUP 03A reconciled")
		if err := c.reconcileEntity(ctx, created, triples, identity, "referenceStaleReconcile"); !errors.Is(err, errEntityConflict) {
			t.Fatalf("stale reconcile: %v", err)
		}
		if err := c.deleteExactEntity(ctx, created, identity); !errors.Is(err, errEntityConflict) {
			t.Fatalf("stale delete: %v", err)
		}
		unchanged, err := c.fetchEntityExact(ctx, id)
		if err != nil || unchanged.KVRevision != reconciled.KVRevision {
			t.Fatalf("rejected stale operations changed authority: %+v %v", unchanged, err)
		}
		if err := c.deleteExactEntity(ctx, reconciled, identity); err != nil {
			t.Fatal(err)
		}
		if _, err := c.fetchEntityExact(ctx, id); !errors.Is(err, errEntityNotFound) {
			t.Fatalf("deleted entity remains readable: %v", err)
		}
	})

	// Failure injection is at the real NATS reply boundary. It does not claim
	// to induce an upstream storage ambiguity; it pins the consumer's response.
	for _, failure := range setup03AExpected(t).Failures {
		for _, operation := range []string{graphMutationCreateOp, graphMutationReconcileOp, graphMutationDeleteOp} {
			t.Run(operation+"/"+failure.Code, func(t *testing.T) {
				server := startEmbeddedNATSServer(t, false)
				responder, err := nats.Connect(server.ClientURL())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(responder.Close)
				var attempts atomic.Int32
				received := make(chan struct{}, 1)
				body, header := encodeEntityMutationFailure(t, failure.Code, "reference failure")
				_, err = responder.Subscribe(mustTestGraphMutationSubject(operation), func(msg *nats.Msg) {
					attempts.Add(1)
					select {
					case received <- struct{}{}:
					default:
					}
					if failure.Injection != "lost_reply" {
						_ = msg.RespondMsg(&nats.Msg{Data: body, Header: header})
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := responder.Flush(); err != nil {
					t.Fatal(err)
				}
				c, err := New(DefaultConfig(), connectSemStreamsClient(t, server.ClientURL()), nil)
				if err != nil {
					t.Fatal(err)
				}
				id, triples, err := c.buildSystemTriplesFromSensorML(setup03AFixture(t, "system.json"))
				if err != nil {
					t.Fatal(err)
				}
				exact := graph.ExactEntity{Entity: &graph.EntityState{ID: id, Triples: triples, MessageType: systemProjectionMessageType}, KVRevision: 42}
				requestCtx, cancelRequest := context.WithCancel(t.Context())
				defer cancelRequest()
				done := make(chan error, 1)
				go func() {
					switch operation {
					case graphMutationCreateOp:
						done <- c.ingestProjectedTriples(requestCtx, id, triples, systemProjectionMessageType, Identity{})
					case graphMutationReconcileOp:
						done <- c.reconcileEntity(requestCtx, exact, triples, Identity{}, "referenceFailure")
					case graphMutationDeleteOp:
						done <- c.deleteExactEntity(requestCtx, exact, Identity{})
					}
				}()
				if failure.Injection == "lost_reply" {
					select {
					case <-received:
						cancelRequest()
					case early := <-done:
						t.Fatalf("request ended before responder receipt: %v", early)
					case <-time.After(10 * time.Second):
						t.Fatal("responder did not receive the request")
					}
				}
				select {
				case err = <-done:
				case <-time.After(10 * time.Second):
					t.Fatal("mutation did not finish within watchdog")
				}
				if err == nil {
					t.Fatal("injected failure was accepted")
				}
				rr := httptest.NewRecorder()
				c.writeBackendError(rr, err)
				if rr.Code != failure.HTTPStatus || attempts.Load() != failure.Attempts {
					t.Fatalf("status=%d attempts=%d, want status=%d attempts=%d", rr.Code, attempts.Load(), failure.HTTPStatus, failure.Attempts)
				}
				if got := rr.Header().Get("X-CS-Commit-Uncertain") == "true"; got != failure.Uncertain {
					t.Fatalf("uncertain=%v want %v", got, failure.Uncertain)
				}
				if failure.Uncertain && rr.Header().Get("X-CS-Correlation-ID") == "" {
					t.Fatal("uncertain outcome lost correlation")
				}
			})
		}
	}
}

func TestSetup03AReferenceImmutableArtifact(t *testing.T) {
	c, client := setup03AGraph(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	store, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{Bucket: c.cfg.SchemaArtifactsBucket})
	if err != nil {
		t.Fatal(err)
	}
	var objectStore schemaObjectStore = store
	c.schemaArtifacts.Store(&objectStore)
	expected := setup03AExpected(t)
	raw := bytes.TrimSpace(setup03AFixture(t, "schema.json"))
	parent := c.cfg.DatastreamIDPrefix + ".setup03a-reference"
	link, err := c.createSchemaArtifact(ctx, parent, csapivocab.HasResultSchema, raw, Identity{})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	wantID := fmt.Sprintf("%s.sha256-%x", c.cfg.SchemaArtifactIDPrefix, digest)
	if link.Subject != parent || link.Object != wantID || link.Datatype != message.EntityReferenceDatatype {
		t.Fatalf("artifact link: %+v want %s", link, wantID)
	}
	exact, err := c.fetchEntityExact(ctx, wantID)
	if err != nil || exact.Entity == nil || exact.KVRevision == 0 {
		t.Fatalf("artifact authority: %+v %v", exact, err)
	}
	wantRef := &message.StorageReference{StorageInstance: expected.StorageInstance, Key: fmt.Sprintf("sha256-%x.json", digest), ContentType: expected.ContentType, Size: int64(len(raw))}
	if !storageReferencesEqual(exact.Entity.StorageRef, wantRef) {
		t.Fatalf("storage reference: %+v want %+v", exact.Entity.StorageRef, wantRef)
	}
	requireStoredTriple(t, exact, sensorml.PredType, csapivocab.SWESchemaDocument)
	firstInfo, err := store.GetInfo(ctx, wantRef.Key)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{raw, append([]byte("\n  "), raw...)} {
		reused, err := c.createSchemaArtifact(ctx, parent, csapivocab.HasResultSchema, input, Identity{})
		if err != nil || reused.Object != wantID {
			t.Fatalf("canonical artifact reuse: %+v %v", reused, err)
		}
	}
	changed := strings.Replace(string(raw), "Cel", "K", 1)
	newLink, err := c.createSchemaArtifact(ctx, parent, csapivocab.HasResultSchema, json.RawMessage(changed), Identity{})
	if err != nil || newLink.Object == wantID {
		t.Fatalf("changed schema must create new artifact: %+v %v", newLink, err)
	}
	got, found, err := c.readSchemaArtifact(ctx, []message.Triple{link}, csapivocab.HasResultSchema)
	if err != nil || !found || !bytes.Equal(got, raw) {
		t.Fatalf("original canonical bytes changed: %s %v %v", got, found, err)
	}
	lastInfo, err := store.GetInfo(ctx, wantRef.Key)
	if err != nil || firstInfo.NUID != lastInfo.NUID || !firstInfo.ModTime.Equal(lastInfo.ModTime) {
		t.Fatalf("original object was rewritten: before=%+v after=%+v error=%v", firstInfo, lastInfo, err)
	}
	unchanged, err := c.fetchEntityExact(ctx, wantID)
	if err != nil || unchanged.KVRevision != exact.KVRevision {
		t.Fatalf("original graph artifact was mutated: %+v %v", unchanged, err)
	}
}
