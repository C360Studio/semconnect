package csapi

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/projection"
)

func TestProjectionContractsValidateAndCoverEveryResourceFamily(t *testing.T) {
	contracts := projectionContracts(DefaultConfig())
	if got, want := len(contracts), 11; got != want {
		t.Fatalf("contracts: got %d want %d", got, want)
	}
	if err := projection.ValidateContracts(contracts); err != nil {
		t.Fatalf("validate projection contracts: %v", err)
	}
	for _, contract := range contracts {
		if !contract.MessageType.IsValid() {
			t.Errorf("contract %q has invalid message type %q", contract.Name, contract.MessageType)
		}
		foundType := false
		for _, predicate := range contract.BirthPredicates {
			foundType = foundType || predicate == sensorml.PredType
		}
		if !foundType {
			t.Errorf("contract %q does not declare type birth-only", contract.Name)
		}
	}
}

func TestProjectionMutationRejectsPatternAndUndeclaredPredicatesBeforeNATS(t *testing.T) {
	validID := DefaultConfig().SystemIDPrefix + ".alpha"
	valid := []message.Triple{{Subject: validID, Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"}}
	tests := []struct {
		name  string
		id    string
		facts []message.Triple
	}{
		{
			name:  "create pattern",
			id:    "acme.ops.robotics.gcs.system.alpha",
			facts: []message.Triple{{Subject: "acme.ops.robotics.gcs.system.alpha", Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"}},
		},
		{
			name: "create undeclared predicate",
			id:   validID,
			facts: append(append([]message.Triple(nil), valid...), message.Triple{
				Subject: validID, Predicate: "cs-api.system.undeclared", Object: "bad",
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			c := newTestComponent(t, fake)
			err := c.createProjectedEntity(context.Background(), systemProjectionContractName,
				tt.id, tt.facts, systemProjectionMessageType, nil, Identity{}, "testCreate")
			if err == nil {
				t.Fatal("invalid projection was accepted")
			}
			if fake.gotSubject != "" {
				t.Fatalf("NATS called for invalid projection: %q", fake.gotSubject)
			}
		})
	}

	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)
	current := graph.EntityState{ID: validID, MessageType: systemProjectionMessageType, Triples: valid}
	desired := append(append([]message.Triple(nil), valid...), message.Triple{
		Subject: validID, Predicate: "cs-api.system.undeclared", Object: "bad",
	})
	if err := c.reconcileEntity(context.Background(), graph.ExactEntity{Entity: &current, KVRevision: 7}, desired, Identity{}, "testReconcile"); err == nil {
		t.Fatal("reconcile silently filtered undeclared predicate")
	}
	if fake.batchCount != 0 {
		t.Fatalf("NATS called for invalid reconcile: %d", fake.batchCount)
	}
}

func TestProjectionContractsAcceptActualBuilderOutputs(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})
	systemID := c.cfg.SystemIDPrefix + ".parent"
	controlStreamID := c.cfg.ControlStreamIDPrefix + ".ptz"
	type builtProjection struct {
		contract string
		id       string
		triples  []message.Triple
		err      error
	}
	build := func(contract string, fn func() (string, []message.Triple, error)) builtProjection {
		id, triples, err := fn()
		return builtProjection{contract: contract, id: id, triples: triples, err: err}
	}
	withRelationship := func(id string, triples []message.Triple, err error, predicate string) (string, []message.Triple, error) {
		if err == nil {
			triples = append(triples, message.Triple{
				Subject: id, Predicate: predicate, Object: c.cfg.SchemaArtifactIDPrefix + ".sha256-test",
				Datatype: message.EntityReferenceDatatype,
			})
		}
		return id, triples, err
	}

	datastreamID := c.cfg.DatastreamIDPrefix + ".temperature"
	datastreamTriples := datastreamToTriples(datastreamID, &Datastream{
		Name: "Temperature", Description: "Ambient temperature", System: systemID,
		ObservedProperty: "https://example.test/property/temperature",
		PhenomenonTime:   "2026-08-12T00:00:00Z/2026-08-12T01:00:00Z",
		ResultTime:       "2026-08-12T01:00:00Z",
	})
	datastreamTriples = append(datastreamTriples, message.Triple{
		Subject: datastreamID, Predicate: csapivocab.HasResultSchema,
		Object: c.cfg.SchemaArtifactIDPrefix + ".sha256-result", Datatype: message.EntityReferenceDatatype,
	})

	projections := []builtProjection{
		build(systemProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildSystemTriplesFromFeature([]byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[-97,30,200]},"properties":{"uid":"system-child","name":"System","description":"desc","parent@id":"` + systemID + `"}}`))
		}),
		{contract: datastreamProjectionContractName, id: datastreamID, triples: datastreamTriples},
		build(procedureProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildProcedureTriplesFromFeature([]byte(`{"type":"Feature","properties":{"uid":"procedure","name":"Procedure","description":"desc","definition":"https://example.test/procedure"}}`))
		}),
		build(deploymentProjectionContractName, func() (string, []message.Triple, error) {
			parent := c.cfg.DeploymentIDPrefix + ".parent"
			return c.buildDeploymentTriplesFromFeature([]byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[-97,30,200]},"properties":{"uid":"deployment-child","name":"Deployment","description":"desc","parent@id":"` + parent + `","deployedSystems@link":[{"href":"/systems/` + systemID + `"}]}}`))
		}),
		build(samplingFeatureProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildSamplingFeatureTriplesFromFeature([]byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[-97,30,200]},"properties":{"uid":"sample","name":"Sample","description":"desc","hostedProcedure@link":{"href":"/procedures/` + c.cfg.ProcedureIDPrefix + `.procedure"}}}`))
		}),
		build(propertyProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildPropertyTriples([]byte(`{"uniqueId":"temperature","label":"Temperature","description":"desc","definition":"https://example.test/property/temperature","baseProperty":"https://example.test/property/base"}`))
		}),
		build(controlStreamProjectionContractName, func() (string, []message.Triple, error) {
			id, triples, _, err := c.buildControlStreamTriples([]byte(`{"id":"` + controlStreamID + `","name":"PTZ","description":"desc","system@id":"` + systemID + `","inputName":"ptz","issueTime":"2026-08-12T00:00:00Z","executionTime":"2026-08-12T00:01:00Z","async":true,"schema":{"commandFormat":"application/json","parametersSchema":{"type":"DataRecord","fields":[{"name":"pan","type":"Quantity"}]}},"controlledProperties":[{"definition":"https://example.test/property/pan"}]}`))
			return withRelationship(id, triples, err, csapivocab.HasCommandSchema)
		}),
		build(commandProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildCommandTriples([]byte(`{"controlstream@id":"` + controlStreamID + `","issueTime":"2026-08-12T00:00:00Z","executionTime":"2026-08-12T00:01:00Z","status":"accepted","sender":"operator","params":{"pan":10}}`))
		}),
		build(systemEventProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildSystemEventTriples([]byte(`{"system@id":"`+systemID+`","systemUid":"system-parent","time":"2026-08-12T00:00:00Z","eventType":"status","message":"ready","description":"desc","severity":"info","source":"test","keywords":["ready"],"payload":{"ok":true}}`), "")
		}),
		build(feasibilityProjectionContractName, func() (string, []message.Triple, error) {
			return c.buildFeasibilityTriples([]byte(`{"controlstream@id":"` + controlStreamID + `","status":"feasible","params":{"pan":10},"result":{"accepted":true}}`))
		}),
		{
			contract: schemaArtifactProjectionContractName,
			id:       c.cfg.SchemaArtifactIDPrefix + ".sha256-test",
			triples: []message.Triple{{
				Subject:   c.cfg.SchemaArtifactIDPrefix + ".sha256-test",
				Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument,
			}},
		},
	}

	contracts := make(map[string]projection.Contract)
	for _, contract := range projectionContracts(c.cfg) {
		contracts[contract.Name] = contract
	}
	for _, built := range projections {
		t.Run(built.contract, func(t *testing.T) {
			if built.err != nil {
				t.Fatalf("build representative projection: %v", built.err)
			}
			contract, ok := contracts[built.contract]
			if !ok {
				t.Fatalf("projection contract %q missing", built.contract)
			}
			if err := validateProjectionFacts(contract, built.id, built.triples); err != nil {
				t.Fatalf("actual builder output violates contract: %v\ntriples=%+v", err, built.triples)
			}
		})
	}
}

func TestMutableProjectionContractsUseOneAtomicRepresentationGroup(t *testing.T) {
	contracts := projectionContracts(DefaultConfig())
	for _, contract := range contracts[:2] {
		if len(contract.Groups) != 1 || contract.Groups[0].Name != "representation" || contract.Groups[0].Mode != projection.ModeReconcile {
			t.Errorf("contract %q groups: %+v", contract.Name, contract.Groups)
		}
	}
	if got := contracts[1].EntityPattern; got != "*.*.*.*.*.*" {
		t.Errorf("Datastream entity pattern: got %q", got)
	}
}

func TestBeta160PortDeclarationsAreClosedAndMutationSubjectsAreDerived(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})
	if len(c.InputPorts()) != 0 {
		t.Fatalf("synthetic input ports remain: %+v", c.InputPorts())
	}
	definitions := c.outputPortDefinitions()
	mutationProviders := 0
	for _, definition := range definitions {
		if strings.Contains(definition.Name, "http") || strings.Contains(definition.Name, "objectstore") {
			t.Errorf("removed custom port remains: %+v", definition)
		}
		request, ok := definition.Config.(component.NATSRequestPort)
		if ok && request.Subject == graphMutationSubjectFamily {
			mutationProviders++
			if !definition.Required || request.Interface == nil ||
				request.Interface.Type != graphMutationInterfaceType || request.Interface.Version != graphMutationInterfaceVer {
				t.Errorf("typed mutation declaration drifted: %+v", definition)
			}
		}
	}
	if mutationProviders != 1 {
		t.Fatalf("typed mutation providers: got %d want 1", mutationProviders)
	}
	for operation, want := range map[string]string{
		graphMutationCreateOp:    "graph.mutation.entity.create",
		graphMutationReconcileOp: "graph.mutation.entity.reconcile",
		graphMutationDeleteOp:    "graph.mutation.entity.delete",
	} {
		got, err := c.graphMutationSubject(operation)
		if err != nil || got != want {
			t.Errorf("resolve %s: got %q err=%v want %q", operation, got, err, want)
		}
	}
	if got := len(c.OutputPorts()); got != len(definitions) {
		t.Fatalf("resolved output ports: got %d want %d", got, len(definitions))
	}
}
