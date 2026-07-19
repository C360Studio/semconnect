//go:build integration

package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	semcomponent "github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestBeta153StructuralMutationContract drives semconnect's real System
// projection and adversarial requests through beta.153 graph-ingest over live
// NATS. Local preflight validation is valuable, but this test proves the shared
// persistence choke point independently rejects malformed direct callers.
func TestBeta153StructuralMutationContract(t *testing.T) {
	ctx := t.Context()
	testNATS := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testNATS.Client

	gateway, err := New(DefaultConfig(), client, nil)
	if err != nil {
		t.Fatalf("create CS API component: %v", err)
	}
	if err := gateway.bindProjectionContracts(ctx); err != nil {
		t.Fatalf("bind System projection contract: %v", err)
	}
	store := startBeta153GraphIngest(t, ctx, client, testNATS.GetNativeConnection())

	fixture := filepath.Join("..", "..", "conformance", "fixtures", "system-hosted.sml.json")
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read hosted-System fixture: %v", err)
	}
	parentID, triples, err := gateway.buildSystemTriplesFromSensorML(body)
	if err != nil {
		t.Fatalf("project hosted-System fixture: %v", err)
	}

	create := graph.CreateEntityWithTriplesRequest{
		Entity:  &graph.EntityState{ID: parentID, MessageType: systemProjectionMessageType},
		Triples: triples,
	}
	requestMutation(t, ctx, client, graphingest.SubjectEntityCreateWithTriples, create)
	parent := getKVEntry(t, ctx, store.kv, parentID)
	var parentState graph.EntityState
	if err := graph.UnmarshalEntityState(parent.Value(), &parentState); err != nil {
		t.Fatalf("validate stored parent: %v", err)
	}

	childID := ""
	for _, triple := range triples {
		if triple.Subject != parentID && triple.Predicate == "sensorml.component.is-hosted-by" {
			childID = triple.Subject
			break
		}
	}
	if childID == "" {
		t.Fatal("hosted-System fixture did not produce the required foreign edge")
	}
	child := getKVEntry(t, ctx, store.kv, childID)
	var childState graph.EntityState
	if err := graph.UnmarshalEntityState(child.Value(), &childState); err != nil {
		t.Fatalf("validate stored hosted child: %v", err)
	}
	if !childState.IsStub() {
		t.Fatalf("claimed no-birth foreign edge did not materialize an envelope-bearing stub: %+v", childState)
	}
	if !hasExactTriple(childState.Triples, childID, "sensorml.component.is-hosted-by", parentID) {
		t.Fatalf("claimed foreign edge was not routed onto hosted child: %+v", childState.Triples)
	}

	validUpdate := graph.UpdateEntityWithTriplesRequest{
		Entity: &graph.EntityState{ID: parentID},
		AddTriples: []message.Triple{{
			Subject: parentID, Predicate: "sensorml.process.label", Object: "beta.153 validated",
		}},
	}
	requestMutation(t, ctx, client, graphingest.SubjectEntityUpdateWithTriples, validUpdate)
	updated := getKVEntry(t, ctx, store.kv, parentID)
	var updatedState graph.EntityState
	if err := graph.UnmarshalEntityState(updated.Value(), &updatedState); err != nil {
		t.Fatalf("validate updated parent: %v", err)
	}
	if !hasExactTriple(updatedState.Triples, parentID, "sensorml.process.label", "beta.153 validated") {
		t.Fatalf("valid update was not persisted unchanged: %+v", updatedState.Triples)
	}

	t.Run("poison is scoped to one entity and out-of-band repair recovers without restart", func(t *testing.T) {
		const isolatedID = "c360.semconnect.systems.csapi.system.isolated"
		requestMutation(t, ctx, client, graphingest.SubjectEntityCreateWithTriples,
			graph.CreateEntityWithTriplesRequest{
				Entity: &graph.EntityState{ID: isolatedID, MessageType: systemProjectionMessageType},
				Triples: []message.Triple{{
					Subject: isolatedID, Predicate: "sensorml.process.label", Object: "beta.153 isolated",
				}},
			})

		canonical := getKVEntry(t, ctx, store.kv, isolatedID)
		canonicalBytes := bytes.Clone(canonical.Value())
		poison := graph.EntityState{
			ID:          isolatedID,
			MessageType: systemProjectionMessageType,
			Triples: []message.Triple{{
				Subject: isolatedID, Predicate: "Invalid.Resident.Predicate", Object: "poison",
			}},
		}
		poisonBytes, err := json.Marshal(poison)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.kv.Put(ctx, isolatedID, poisonBytes); err != nil {
			t.Fatalf("poison isolated entity out-of-band: %v", err)
		}

		parentBytes, err := queryEntity(t, ctx, client, parentID)
		if err != nil {
			t.Fatalf("healthy entity %s became unreadable after poisoning %s: %v", parentID, isolatedID, err)
		}
		var healthy graph.EntityState
		if err := graph.UnmarshalEntityState(parentBytes, &healthy); err != nil {
			t.Fatalf("decode healthy entity %s: %v", parentID, err)
		}
		if healthy.ID != parentID {
			t.Fatalf("healthy query returned entity %q, want %q", healthy.ID, parentID)
		}

		_, err = queryEntity(t, ctx, client, isolatedID)
		var classified *errs.ClassifiedError
		if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal ||
			classified.Code != graph.ErrorCodeGraphStateResetRequired {
			t.Fatalf("poisoned entity query = %v, want fatal %s classification",
				err, graph.ErrorCodeGraphStateResetRequired)
		}
		_, err = queryEntities(t, ctx, client, isolatedID)
		if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal ||
			classified.Code != graph.ErrorCodeGraphStateResetRequired {
			t.Fatalf("poisoned aggregate query = %v, want fatal %s classification",
				err, graph.ErrorCodeGraphStateResetRequired)
		}
		if !strings.Contains(err.Error(), isolatedID) {
			t.Fatalf("poisoned aggregate error %q does not name scoped entity %s", err, isolatedID)
		}

		if _, err := store.kv.Put(ctx, isolatedID, canonicalBytes); err != nil {
			t.Fatalf("repair isolated entity out-of-band: %v", err)
		}
		repairedBytes, err := queryEntity(t, ctx, client, isolatedID)
		if err != nil {
			t.Fatalf("out-of-band repaired entity did not recover without graph-ingest restart: %v", err)
		}
		var repaired graph.EntityState
		if err := graph.UnmarshalEntityState(repairedBytes, &repaired); err != nil {
			t.Fatalf("decode repaired entity: %v", err)
		}
		if !hasExactTriple(repaired.Triples, isolatedID, "sensorml.process.label", "beta.153 isolated") {
			t.Fatalf("repaired entity did not restore canonical state: %+v", repaired.Triples)
		}
	})

	for _, test := range []struct {
		name   string
		triple message.Triple
	}{
		{
			name:   "predicate",
			triple: message.Triple{Subject: parentID, Predicate: "cs-api.system.camelCase", Object: "invalid"},
		},
		{
			name:   "entity ID",
			triple: message.Triple{Subject: "not-an-entity-id", Predicate: "sensorml.process.label", Object: "invalid"},
		},
		{
			name: "entity reference",
			triple: message.Triple{
				Subject: parentID, Predicate: "sensorml.component.is-hosted-by",
				Object: "not-an-entity-id", Datatype: message.EntityReferenceDatatype,
			},
		},
	} {
		t.Run("reject invalid "+test.name+" atomically", func(t *testing.T) {
			before := getKVEntry(t, ctx, store.kv, parentID)
			beforeBytes := bytes.Clone(before.Value())
			beforeBucketRevision := store.lastRevision(t, ctx)
			req := graph.UpdateEntityWithTriplesRequest{
				Entity:     &graph.EntityState{ID: parentID},
				AddTriples: []message.Triple{test.triple},
			}
			requireInvalidMutation(t, ctx, client, graphingest.SubjectEntityUpdateWithTriples, req)
			after := getKVEntry(t, ctx, store.kv, parentID)
			if after.Revision() != before.Revision() || !bytes.Equal(after.Value(), beforeBytes) {
				t.Fatalf("rejected mutation changed parent: revision %d -> %d", before.Revision(), after.Revision())
			}
			if got := store.lastRevision(t, ctx); got != beforeBucketRevision {
				t.Fatalf("rejected mutation changed bucket revision: %d -> %d", beforeBucketRevision, got)
			}
		})
	}

	t.Run("resident poison is not laundered", func(t *testing.T) {
		const poisonID = "c360.semconnect.systems.csapi.system.poison"
		poison := graph.EntityState{
			ID:          poisonID,
			MessageType: systemProjectionMessageType,
			Triples: []message.Triple{{
				Subject: poisonID, Predicate: "Invalid.Resident.Predicate", Object: "poison",
			}},
		}
		if err := graph.ValidateEntityStateContract(&poison); err == nil {
			t.Fatal("resident-poison fixture unexpectedly satisfies the structural contract")
		}
		raw, err := json.Marshal(poison)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.kv.Put(ctx, poisonID, raw); err != nil {
			t.Fatalf("inject resident poison: %v", err)
		}
		before := getKVEntry(t, ctx, store.kv, poisonID)
		beforeBytes := bytes.Clone(before.Value())
		beforeBucketRevision := store.lastRevision(t, ctx)

		req := graph.UpdateEntityWithTriplesRequest{
			Entity: &graph.EntityState{ID: poisonID},
			AddTriples: []message.Triple{{
				Subject: poisonID, Predicate: "sensorml.process.label", Object: "must not commit",
			}},
		}
		err = mutationError(t, ctx, client, graphingest.SubjectEntityUpdateWithTriples, req)
		var classified *errs.ClassifiedError
		if !errors.As(err, &classified) || classified.Code != graph.ErrorCodeGraphStateResetRequired {
			t.Fatalf("poison rejection = %v, want %s classification", err, graph.ErrorCodeGraphStateResetRequired)
		}
		after := getKVEntry(t, ctx, store.kv, poisonID)
		if after.Revision() != before.Revision() || !bytes.Equal(after.Value(), beforeBytes) {
			t.Fatalf("resident poison was rewritten: revision %d -> %d", before.Revision(), after.Revision())
		}
		if got := store.lastRevision(t, ctx); got != beforeBucketRevision {
			t.Fatalf("poison rejection changed bucket revision: %d -> %d", beforeBucketRevision, got)
		}
	})

	t.Run("remove no-op does not write", func(t *testing.T) {
		before := getKVEntry(t, ctx, store.kv, parentID)
		beforeBytes := bytes.Clone(before.Value())
		beforeBucketRevision := store.lastRevision(t, ctx)
		requestMutation(t, ctx, client, graphingest.SubjectTripleRemove, graph.RemoveTripleRequest{
			Subject: parentID, Predicate: "test.absent.predicate",
		})
		after := getKVEntry(t, ctx, store.kv, parentID)
		if after.Revision() != before.Revision() || !bytes.Equal(after.Value(), beforeBytes) {
			t.Fatalf("no-op remove changed parent: revision %d -> %d", before.Revision(), after.Revision())
		}
		if got := store.lastRevision(t, ctx); got != beforeBucketRevision {
			t.Fatalf("no-op remove changed bucket revision: %d -> %d", beforeBucketRevision, got)
		}
	})
}

type beta153GraphStore struct {
	kv     jetstream.KeyValue
	stream jetstream.Stream
}

func startBeta153GraphIngest(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	rawConnection *nats.Conn,
) beta153GraphStore {
	t.Helper()

	config := graphingest.DefaultConfig()
	config.Ports.Inputs = []semcomponent.PortDefinition{{
		Name: "unused_in", Type: "nats", Subject: "_semconnect.test.unused",
	}}
	rawConfig, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	discoverable, err := graphingest.CreateGraphIngest(rawConfig, semcomponent.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("create graph-ingest: %v", err)
	}
	lifecycle, ok := discoverable.(semcomponent.LifecycleComponent)
	if !ok {
		t.Fatal("graph-ingest does not implement LifecycleComponent")
	}
	if err := lifecycle.Initialize(); err != nil {
		t.Fatalf("initialize graph-ingest: %v", err)
	}
	if err := lifecycle.Start(ctx); err != nil {
		t.Fatalf("start graph-ingest: %v", err)
	}
	t.Cleanup(func() {
		if err := lifecycle.Stop(5 * time.Second); err != nil {
			t.Errorf("stop graph-ingest: %v", err)
		}
	})

	js, err := jetstream.New(rawConnection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	kv, err := js.KeyValue(ctx, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEntityStates, err)
	}
	stream, err := js.Stream(ctx, "KV_"+graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open entity-state stream: %v", err)
	}
	return beta153GraphStore{kv: kv, stream: stream}
}

func (s beta153GraphStore) lastRevision(t *testing.T, ctx context.Context) uint64 {
	t.Helper()
	info, err := s.stream.Info(ctx)
	if err != nil {
		t.Fatalf("read entity-state stream info: %v", err)
	}
	return info.State.LastSeq
}

func getKVEntry(t *testing.T, ctx context.Context, kv jetstream.KeyValue, id string) jetstream.KeyValueEntry {
	t.Helper()
	entry, err := kv.Get(ctx, id)
	if err != nil {
		t.Fatalf("get entity %s: %v", id, err)
	}
	return entry
}

func requestMutation(t *testing.T, ctx context.Context, client *natsclient.Client, subject string, request any) []byte {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.RequestClassified(ctx, subject, payload, 2*time.Second)
	if err != nil {
		t.Fatalf("request %s: %v", subject, err)
	}
	return response
}

func queryEntity(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	entityID string,
) ([]byte, error) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"id": entityID})
	if err != nil {
		t.Fatal(err)
	}
	return client.RequestClassified(ctx, "graph.ingest.query.entity", payload, 2*time.Second)
}

func queryEntities(
	t *testing.T,
	ctx context.Context,
	client *natsclient.Client,
	entityIDs ...string,
) ([]byte, error) {
	t.Helper()
	payload, err := json.Marshal(map[string][]string{"ids": entityIDs})
	if err != nil {
		t.Fatal(err)
	}
	return client.RequestClassified(ctx, "graph.ingest.query.batch", payload, 2*time.Second)
}

func mutationError(t *testing.T, ctx context.Context, client *natsclient.Client, subject string, request any) error {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.RequestClassified(ctx, subject, payload, 2*time.Second)
	if err == nil {
		t.Fatalf("request %s unexpectedly succeeded: %s", subject, response)
	}
	if response != nil {
		t.Fatalf("request %s returned both response and error: %s / %v", subject, response, err)
	}
	return err
}

func requireInvalidMutation(t *testing.T, ctx context.Context, client *natsclient.Client, subject string, request any) {
	t.Helper()
	err := mutationError(t, ctx, client, subject, request)
	if !errs.IsInvalid(err) {
		t.Fatalf("request %s error = %v, want invalid classification", subject, err)
	}
}

func hasExactTriple(triples []message.Triple, subject, predicate, object string) bool {
	for _, triple := range triples {
		if triple.Subject == subject && triple.Predicate == predicate && triple.Object == object {
			return true
		}
	}
	return false
}
