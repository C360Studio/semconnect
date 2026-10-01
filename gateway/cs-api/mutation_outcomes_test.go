package csapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
	"time"
)

type mutationFaultRequester struct {
	natsRequester
	calls int
	reply *nats.Msg
	err   error
}

func (f *mutationFaultRequester) RequestWithHeaders(context.Context, string, []byte, map[string]string, time.Duration) (*nats.Msg, error) {
	f.calls++
	return f.reply, f.err
}

func mutationOutcomeFixture(t *testing.T) (*Component, graph.ExactEntity, *mutationFaultRequester) {
	t.Helper()
	c := newTestComponent(t, nil)
	id := c.cfg.SystemIDPrefix + ".outcomes"
	exact := graph.ExactEntity{Entity: &graph.EntityState{ID: id, MessageType: systemProjectionMessageType,
		Triples: []message.Triple{{Subject: id, Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"}}}, KVRevision: 42}
	fault := &mutationFaultRequester{}
	c.nats = fault
	return c, exact, fault
}

func TestTypedMutationInvalidSuccessRemainsUncertainWithoutRetry(t *testing.T) {
	for _, operation := range []string{"create", "reconcile", "delete"} {
		for _, body := range []string{`{`, `{"outcome":"applied"}`, `{"outcome":"applied","entity":{"id":"wrong"},"entity_id":"wrong","kv_revision":2,"expected_revision":41}`} {
			t.Run(operation+"/"+body, func(t *testing.T) {
				c, exact, fault := mutationOutcomeFixture(t)
				fault.reply = &nats.Msg{Data: []byte(body)}
				var err error
				switch operation {
				case "create":
					err = c.ingestProjectedTriples(t.Context(), exact.Entity.ID, exact.Entity.Triples, systemProjectionMessageType, Identity{})
				case "reconcile":
					err = c.reconcileEntity(t.Context(), exact, exact.Entity.Triples, Identity{}, "outcome")
				case "delete":
					err = c.deleteExactEntity(t.Context(), exact, Identity{})
				}
				if err == nil {
					t.Fatal("unverified success accepted")
				}
				rr := httptest.NewRecorder()
				c.writeBackendError(rr, err)
				if fault.calls != 1 || rr.Code != http.StatusServiceUnavailable || rr.Header().Get("X-CS-Commit-Uncertain") != "true" || rr.Header().Get("X-CS-Correlation-ID") == "" {
					t.Fatalf("uncertain outcome lost: calls=%d status=%d headers=%v", fault.calls, rr.Code, rr.Header())
				}
			})
		}
	}
}

func TestTypedMutationUnavailabilityAndCancellationDoNotClaimUnknownCommit(t *testing.T) {
	for _, test := range []struct {
		name     string
		canceled bool
		err      error
		calls    int
	}{
		{name: "canceled before dispatch", canceled: true},
		{name: "no responder", err: nats.ErrNoResponders, calls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, exact, fault := mutationOutcomeFixture(t)
			fault.err = test.err
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.canceled {
				cancel()
			}
			err := c.ingestProjectedTriples(ctx, exact.Entity.ID, exact.Entity.Triples, systemProjectionMessageType, Identity{})
			if !errs.IsTransient(err) {
				t.Fatalf("unavailable request lost transient classification: %v", err)
			}
			rr := httptest.NewRecorder()
			c.writeBackendError(rr, err)
			if fault.calls != test.calls || rr.Code != http.StatusServiceUnavailable || rr.Header().Get("X-CS-Commit-Uncertain") != "" {
				t.Fatalf("unavailable outcome: calls=%d status=%d headers=%v", fault.calls, rr.Code, rr.Header())
			}
		})
	}
}

func TestTypedMutationRequiresExactNonzeroAuthorityBeforeIO(t *testing.T) {
	for _, absent := range []bool{false, true} {
		c, exact, fault := mutationOutcomeFixture(t)
		triples := exact.Entity.Triples
		exact.KVRevision = 0
		if absent {
			exact.Entity = nil
		}
		if err := c.reconcileEntity(t.Context(), exact, triples, Identity{}, "outcome"); !errs.IsInvalid(err) {
			t.Fatalf("unfenced reconcile: %v", err)
		}
		if err := c.deleteExactEntity(t.Context(), exact, Identity{}); !errs.IsInvalid(err) {
			t.Fatalf("unfenced delete: %v", err)
		}
		if fault.calls != 0 {
			t.Fatalf("unfenced operations reached NATS: %d", fault.calls)
		}
	}
}

func TestTypedCreateRequiresMatchingRegisteredProjection(t *testing.T) {
	for _, test := range []struct {
		name, contract string
		mt             message.Type
	}{
		{name: "invalid message type", contract: systemProjectionContractName},
		{name: "unknown contract", contract: "cs-api.unknown", mt: systemProjectionMessageType},
		{name: "contract type mismatch", contract: systemProjectionContractName, mt: datastreamProjectionMessageType},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, exact, fault := mutationOutcomeFixture(t)
			err := c.createProjectedEntity(t.Context(), test.contract, exact.Entity.ID, exact.Entity.Triples, test.mt, nil, Identity{}, "outcome")
			if !errs.IsInvalid(err) || fault.calls != 0 {
				t.Fatalf("invalid create reached backend: %v calls=%d", err, fault.calls)
			}
		})
	}
}

func TestTypedReconcileCannotExpandOwnedRepresentation(t *testing.T) {
	c, exact, fault := mutationOutcomeFixture(t)
	desired := []message.Triple{{Subject: "foreign.remote.systems.csapi.system.one", Predicate: sensorml.PredLabel, Object: "foreign"}}
	if err := c.reconcileEntity(t.Context(), exact, desired, Identity{}, "outcome"); !errs.IsInvalid(err) || fault.calls != 0 {
		t.Fatalf("foreign reconcile reached backend: %v calls=%d", err, fault.calls)
	}
	// An entity without a representation group cannot gain PATCH/PUT authority.
	exact.Entity.ID = c.cfg.SchemaArtifactIDPrefix + ".one"
	exact.Entity.Triples = []message.Triple{{Subject: exact.Entity.ID, Predicate: sensorml.PredType, Object: "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/SWESchemaDocument"}}
	if err := c.reconcileEntity(t.Context(), exact, exact.Entity.Triples, Identity{}, "outcome"); !errs.IsInvalid(err) || fault.calls != 0 {
		t.Fatalf("immutable entity gained reconcile authority: %v calls=%d", err, fault.calls)
	}
}
