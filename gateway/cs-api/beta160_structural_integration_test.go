//go:build integration

package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	semcomponent "github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
)

// TestBeta160TypedGraphLifecycle runs semconnect's real projection and typed
// mutation adapter through beta.160 graph-ingest over live NATS. It covers the
// authority lifecycle, revision fencing, batch missing identities, and the
// root-only relationship rule without relying on a compatibility subject.
func TestBeta160TypedGraphLifecycle(t *testing.T) {
	ctx := t.Context()
	testNATS := natsclient.NewTestClient(t, natsclient.WithKV())
	client := testNATS.Client

	gateway, err := New(DefaultConfig(), client, nil)
	if err != nil {
		t.Fatalf("create CS API component: %v", err)
	}
	if err := gateway.bindProjectionContracts(ctx); err != nil {
		t.Fatalf("validate projection contracts: %v", err)
	}
	startBeta160GraphIngest(t, ctx, client)
	bridgeBeta160AuthorityQueries(t, ctx, client)

	fixture := filepath.Join("..", "..", "conformance", "fixtures", "system-hosted.sml.json")
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read hosted-System fixture: %v", err)
	}
	parentID, triples, err := gateway.buildSystemTriplesFromSensorML(body)
	if err != nil {
		t.Fatalf("project hosted-System fixture: %v", err)
	}
	childID := ""
	for _, triple := range triples {
		if triple.Subject != parentID {
			t.Fatalf("root-only projection leaked foreign-subject fact: %+v", triple)
		}
		if triple.Predicate == "sensorml.system.hosts" {
			childID, _ = triple.Object.(string)
		}
	}
	if childID == "" {
		t.Fatal("hosted-System fixture lost its root-to-child relationship")
	}

	identity := Identity{Subject: "integration", Verified: true}
	if err := gateway.ingestProjectedTriples(ctx, parentID, triples, systemProjectionMessageType, identity); err != nil {
		t.Fatalf("typed create: %v", err)
	}
	exactCreated, err := gateway.fetchEntityExact(ctx, parentID)
	if err != nil {
		t.Fatalf("exact read after create: %v", err)
	}
	if exactCreated.KVRevision == 0 || exactCreated.Entity == nil || exactCreated.Entity.ID != parentID {
		t.Fatalf("invalid exact create authority: %+v", exactCreated)
	}
	if _, err := gateway.fetchEntityExact(ctx, childID); !errors.Is(err, errEntityNotFound) {
		t.Fatalf("missing relationship target created a stub: %v", err)
	}
	if err := gateway.ingestProjectedTriples(ctx, parentID, triples, systemProjectionMessageType, identity); !errors.Is(err, errEntityConflict) {
		t.Fatalf("strict-create conflict: got %v want errEntityConflict", err)
	}

	desired := append([]message.Triple(nil), exactCreated.Entity.Triples...)
	for index := range desired {
		if desired[index].Predicate == "sensorml.process.label" {
			desired[index].Object = "beta.160 reconciled"
		}
	}
	if err := gateway.reconcileEntity(ctx, exactCreated, desired, identity, "integrationReconcile"); err != nil {
		t.Fatalf("revision-fenced reconcile: %v", err)
	}
	exactReconciled, err := gateway.fetchEntityExact(ctx, parentID)
	if err != nil {
		t.Fatalf("exact read after reconcile: %v", err)
	}
	if exactReconciled.KVRevision <= exactCreated.KVRevision {
		t.Fatalf("reconcile revision did not advance: %d -> %d", exactCreated.KVRevision, exactReconciled.KVRevision)
	}
	if err := gateway.reconcileEntity(ctx, exactCreated, desired, identity, "integrationStaleReconcile"); !errors.Is(err, errEntityConflict) {
		t.Fatalf("stale reconcile: got %v want errEntityConflict", err)
	}

	missingID := "c360.semconnect.systems.csapi.system.integration-missing"
	states, err := gateway.fetchEntitiesBatch(ctx, []string{parentID, missingID})
	if err != nil {
		t.Fatalf("batch exact/missing query: %v", err)
	}
	if len(states) != 1 || states[parentID].ID != parentID {
		t.Fatalf("batch response manufactured or lost identities: %+v", states)
	}
	if _, exists := states[missingID]; exists {
		t.Fatalf("batch response manufactured missing entity %q", missingID)
	}

	if err := gateway.deleteExactEntity(ctx, exactReconciled, identity); err != nil {
		t.Fatalf("revision-fenced delete: %v", err)
	}
	if _, err := gateway.fetchEntityExact(ctx, parentID); !errors.Is(err, errEntityNotFound) {
		t.Fatalf("deleted authority remained readable: %v", err)
	}
}

func startBeta160GraphIngest(t *testing.T, ctx context.Context, client *natsclient.Client) {
	t.Helper()
	config := graphingest.DefaultConfig()
	config.Ports.Inputs = []semcomponent.PortDefinition{
		{
			Name: "unused-entity-input",
			Config: semcomponent.NATSPort{
				Subject: "_semconnect.beta160.integration.unused",
			},
		},
		{
			Name: graphMutationPortName, Required: true,
			Config: semcomponent.NATSRequestPort{
				Subject: graphMutationSubjectFamily,
				Interface: &semcomponent.InterfaceContract{
					Type: graphMutationInterfaceType, Version: graphMutationInterfaceVer,
				},
			},
		},
	}
	rawConfig, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	discoverable, err := graphingest.CreateGraphIngest(rawConfig, semcomponent.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("create beta.160 graph-ingest: %v", err)
	}
	lifecycle, ok := discoverable.(semcomponent.LifecycleComponent)
	if !ok {
		t.Fatal("graph-ingest does not implement LifecycleComponent")
	}
	if err := lifecycle.Initialize(); err != nil {
		t.Fatalf("initialize beta.160 graph-ingest: %v", err)
	}
	if err := lifecycle.Start(ctx); err != nil {
		t.Fatalf("start beta.160 graph-ingest: %v", err)
	}
	t.Cleanup(func() {
		if err := lifecycle.Stop(5 * time.Second); err != nil {
			t.Errorf("stop beta.160 graph-ingest: %v", err)
		}
	})
}

func bridgeBeta160AuthorityQueries(t *testing.T, ctx context.Context, client *natsclient.Client) {
	t.Helper()
	for publicSubject, authoritySubject := range map[string]string{
		subjectEntityQuery: "graph.ingest.query.entity",
		subjectBatchQuery:  "graph.ingest.query.batch",
	} {
		authoritySubject := authoritySubject
		subscription, err := client.SubscribeForRequests(ctx, publicSubject, func(requestCtx context.Context, data []byte) ([]byte, error) {
			return client.RequestClassified(requestCtx, authoritySubject, data, 2*time.Second)
		})
		if err != nil {
			t.Fatalf("bridge %s to %s: %v", publicSubject, authoritySubject, err)
		}
		t.Cleanup(func() {
			if err := subscription.Unsubscribe(); err != nil {
				t.Errorf("unsubscribe authority query bridge: %v", err)
			}
		})
	}
}

func requireStoredTriple(t *testing.T, exact graph.ExactEntity, predicate, object string) {
	t.Helper()
	if exact.Entity == nil {
		t.Fatal("exact entity is nil")
	}
	for _, triple := range exact.Entity.Triples {
		if triple.Predicate == predicate && triple.Object == object {
			return
		}
	}
	t.Fatalf("missing triple %q = %q in %+v", predicate, object, exact.Entity.Triples)
}
