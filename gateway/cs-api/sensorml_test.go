package csapi

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// Predicate-set fixtures use the sole canonical internal type predicate.
var (
	typePredCanon  = sensorml.PredType
	labelPred      = sensorml.PredLabel
	descPred       = sensorml.PredDescription
	defPred        = sensorml.PredDefinition
	hostsPred      = sensorml.PredHosts
	hostedByPred   = sensorml.PredIsHostedBy
	subSystemPred  = sensorml.PredHasSubSystem
	procPred       = sensorml.PredUsedProcedure
	attachedPred   = sensorml.PredAttachedTo
	identifierPred = sensorml.PredIdentifierValue
	capPred        = sensorml.PredCapabilityValue
	charPred       = sensorml.PredCharacteristicValue
)

func TestReconstruct_PhysicalSystem(t *testing.T) {
	triples := []message.Triple{
		{Subject: "drone.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
		{Subject: "drone.001", Predicate: labelPred, Object: "ACME Drone 001"},
		{Subject: "drone.001", Predicate: descPred, Object: "Hex rotor with 4K camera"},
		{Subject: "drone.001", Predicate: hostsPred, Object: "drone.001.camera", Datatype: message.EntityReferenceDatatype},
		{Subject: "drone.001", Predicate: hostsPred, Object: "drone.001.battery", Datatype: message.EntityReferenceDatatype},
		{Subject: "drone.001", Predicate: attachedPred, Object: "http://example.org/platforms/uav"},
	}

	proc, err := reconstructProcessFromTriples(triples, "drone.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	sys, ok := proc.(*sensorml.PhysicalSystem)
	if !ok {
		t.Fatalf("type: got %T want *sensorml.PhysicalSystem", proc)
	}
	if sys.ID != "drone.001" {
		t.Errorf("ID: got %q want %q", sys.ID, "drone.001")
	}
	if sys.Label != "ACME Drone 001" {
		t.Errorf("Label: got %q", sys.Label)
	}
	if sys.Description != "Hex rotor with 4K camera" {
		t.Errorf("Description: got %q", sys.Description)
	}
	if sys.AttachedTo == nil || sys.AttachedTo.Href != "http://example.org/platforms/uav" {
		t.Errorf("AttachedTo: got %+v", sys.AttachedTo)
	}
	if len(sys.Components) != 2 {
		t.Fatalf("Components: got %d want 2", len(sys.Components))
	}
	if sys.Components[0].LocalID() != "drone.001.camera" || sys.Components[1].LocalID() != "drone.001.battery" {
		t.Errorf("Components order: got [%q, %q]", sys.Components[0].LocalID(), sys.Components[1].LocalID())
	}
	if _, ok := sys.Components[0].(*sensorml.PhysicalComponent); !ok {
		t.Errorf("Component[0] kind: got %T", sys.Components[0])
	}
}

func TestReconstruct_PhysicalComponent(t *testing.T) {
	triples := []message.Triple{
		{Subject: "drone.001.camera", Predicate: typePredCanon, Object: sosa.Sensor},
		{Subject: "drone.001.camera", Predicate: labelPred, Object: "Forward Camera"},
		{Subject: "drone.001.camera", Predicate: procPred, Object: "http://example.org/procedures/4k-imaging"},
		{Subject: "drone.001.camera", Predicate: hostedByPred, Object: "drone.001", Datatype: message.EntityReferenceDatatype},
		{Subject: "drone.001.camera", Predicate: identifierPred, Object: "SN-2026-A2391"},
	}

	proc, err := reconstructProcessFromTriples(triples, "drone.001.camera")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	pc, ok := proc.(*sensorml.PhysicalComponent)
	if !ok {
		t.Fatalf("type: got %T want *sensorml.PhysicalComponent", proc)
	}
	if pc.Method == nil || pc.Method.Href != "http://example.org/procedures/4k-imaging" {
		t.Errorf("Method: got %+v", pc.Method)
	}
	if len(pc.Identifiers) != 1 || pc.Identifiers[0].Value != "SN-2026-A2391" {
		t.Errorf("Identifiers: got %+v", pc.Identifiers)
	}
}

func TestReconstruct_RejectsRDFTypeCompatibilityRead(t *testing.T) {
	triples := []message.Triple{{
		Subject:   "drone.001.camera",
		Predicate: "rdf.type",
		Object:    sosa.Sensor,
	}}
	if _, err := reconstructProcessFromTriples(triples, "drone.001.camera"); err == nil {
		t.Fatal("rdf.type compatibility state must not reconstruct")
	}
}

func TestReconstruct_SimpleProcess(t *testing.T) {
	triples := []message.Triple{
		{Subject: "proc.calibration", Predicate: typePredCanon, Object: sosa.Procedure},
		{Subject: "proc.calibration", Predicate: labelPred, Object: "Calibration Routine"},
		{Subject: "proc.calibration", Predicate: procPred, Object: "http://example.org/procedures/calibration"},
		// no hasSubSystem → SimpleProcess
	}
	proc, err := reconstructProcessFromTriples(triples, "proc.calibration")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	sp, ok := proc.(*sensorml.SimpleProcess)
	if !ok {
		t.Fatalf("type: got %T want *sensorml.SimpleProcess", proc)
	}
	if sp.Method == nil || sp.Method.Href != "http://example.org/procedures/calibration" {
		t.Errorf("Method: got %+v", sp.Method)
	}
}

func TestReconstruct_AggregateProcessDisambiguatesViaSubSystem(t *testing.T) {
	// Same rdf:type IRI (sosa:Procedure) as SimpleProcess; disambiguated
	// solely by presence of hasSubSystem triples.
	triples := []message.Triple{
		{Subject: "pipeline.weather", Predicate: typePredCanon, Object: sosa.Procedure},
		{Subject: "pipeline.weather", Predicate: subSystemPred, Object: "step.parse", Datatype: message.EntityReferenceDatatype},
		{Subject: "pipeline.weather", Predicate: subSystemPred, Object: "step.smooth", Datatype: message.EntityReferenceDatatype},
	}
	proc, err := reconstructProcessFromTriples(triples, "pipeline.weather")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	agg, ok := proc.(*sensorml.AggregateProcess)
	if !ok {
		t.Fatalf("type: got %T want *sensorml.AggregateProcess", proc)
	}
	if len(agg.Components) != 2 {
		t.Errorf("Components: got %d want 2", len(agg.Components))
	}
}

func TestReconstruct_RejectsMissingType(t *testing.T) {
	// An entity with no rdf:type triple cannot be mapped to a SensorML
	// process kind. Surface as a clear error so the handler can 406.
	triples := []message.Triple{
		{Subject: "drone.001", Predicate: labelPred, Object: "Unlabeled thing"},
	}
	if _, err := reconstructProcessFromTriples(triples, "drone.001"); err == nil {
		t.Errorf("expected error for missing rdf:type, got nil")
	}
}

func TestReconstruct_RejectsUnsupportedType(t *testing.T) {
	// rdf:type IRI outside the four CS-API-critical-path kinds — graceful
	// error rather than silently emitting a wrong SensorML doc.
	triples := []message.Triple{
		{Subject: "x.y", Predicate: typePredCanon, Object: "http://example.org/some/exotic/type"},
	}
	if _, err := reconstructProcessFromTriples(triples, "x.y"); err == nil {
		t.Errorf("expected error for unsupported rdf:type, got nil")
	}
}

// TestReconstruct_LossyRoundTrip property-asserts the round trip
// sensorml.Asset.Triples → reconstructProcessFromTriples produces a Process
// whose CS-API-critical fields match the original. Lossy fields per the
// sensorml.go file doc (inputs/outputs, keywords, connections, identifier
// metadata) are intentionally not compared.
func TestReconstruct_LossyRoundTrip(t *testing.T) {
	original := &sensorml.PhysicalSystem{
		AbstractProcess: sensorml.AbstractProcess{
			ID:          "acme.ops.robotics.gcs.drone.001",
			Label:       "ACME Drone 001",
			Description: "Hex rotor with 4K camera",
			Definition:  "http://example.org/types/quadcopter",
			Identifiers: []sensorml.Term{
				{Value: "SN-2026-A2391"},
			},
		},
		AttachedTo: &sensorml.Reference{Href: "http://example.org/platforms/uav"},
		Components: []sensorml.Process{
			&sensorml.PhysicalComponent{
				AbstractProcess: sensorml.AbstractProcess{ID: "drone.001.camera"},
			},
			&sensorml.PhysicalComponent{
				AbstractProcess: sensorml.AbstractProcess{ID: "drone.001.battery"},
			},
		},
	}
	asset := sensorml.NewAsset(original.ID, original)
	triples := asset.Triples()

	reconstructed, err := reconstructProcessFromTriples(triples, original.ID)
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	got, ok := reconstructed.(*sensorml.PhysicalSystem)
	if !ok {
		t.Fatalf("type: got %T", reconstructed)
	}
	if got.ID != original.ID {
		t.Errorf("ID: got %q want %q", got.ID, original.ID)
	}
	if got.Label != original.Label {
		t.Errorf("Label: got %q want %q", got.Label, original.Label)
	}
	if got.Description != original.Description {
		t.Errorf("Description: got %q want %q", got.Description, original.Description)
	}
	if got.Definition != original.Definition {
		t.Errorf("Definition: got %q want %q", got.Definition, original.Definition)
	}
	if got.AttachedTo == nil || got.AttachedTo.Href != original.AttachedTo.Href {
		t.Errorf("AttachedTo: got %+v want %+v", got.AttachedTo, original.AttachedTo)
	}
	if len(got.Components) != len(original.Components) {
		t.Errorf("Components count: got %d want %d", len(got.Components), len(original.Components))
	}
	// Identifier.Value survives; .Label / .Definition are lossy (only Value
	// is emitted by Asset.Triples). Document with a focused assertion.
	if len(got.Identifiers) != 1 || got.Identifiers[0].Value != "SN-2026-A2391" {
		t.Errorf("Identifier.Value: got %+v want value=SN-2026-A2391", got.Identifiers)
	}
}

func TestReconstruct_MarshalsToValidSensorMLJSON(t *testing.T) {
	// End-to-end smoke: reconstructed PhysicalSystem marshals via the
	// framework's MarshalJSON and the result decodes back via
	// sensorml.UnmarshalProcess. Pins the SensorML JSON wire shape.
	triples := []message.Triple{
		{Subject: "drone.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
		{Subject: "drone.001", Predicate: labelPred, Object: "ACME Drone 001"},
		{Subject: "drone.001", Predicate: hostsPred, Object: "drone.001.camera", Datatype: message.EntityReferenceDatatype},
	}
	proc, err := reconstructProcessFromTriples(triples, "drone.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	body, err := json.Marshal(proc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Round-trip back through the framework parser to confirm shape.
	roundtrip, err := sensorml.UnmarshalProcess(body)
	if err != nil {
		t.Fatalf("framework re-parse: %v; body=%s", err, body)
	}
	if roundtrip.Type() != sensorml.TypePhysicalSystem {
		t.Errorf("Type: got %q want %q", roundtrip.Type(), sensorml.TypePhysicalSystem)
	}
}

func TestReconstruct_NumericIdentifierValuePreserved(t *testing.T) {
	// Identifier.Value is typed `any` — operators may emit integer serial
	// numbers, float timestamps, etc. The reverse mapping must NOT coerce
	// these to strings (would corrupt the type signal).
	triples := []message.Triple{
		{Subject: "drone.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
		{Subject: "drone.001", Predicate: identifierPred, Object: 42},      // int identifier
		{Subject: "drone.001", Predicate: identifierPred, Object: "SN-A1"}, // string identifier
		{Subject: "drone.001", Predicate: identifierPred, Object: 3.14},    // float identifier
	}
	proc, err := reconstructProcessFromTriples(triples, "drone.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	idents := proc.Base().Identifiers
	if len(idents) != 3 {
		t.Fatalf("Identifiers: got %d want 3", len(idents))
	}
	if v, ok := idents[0].Value.(int); !ok || v != 42 {
		t.Errorf("Identifier[0]: got %T(%v) want int(42)", idents[0].Value, idents[0].Value)
	}
	if v, ok := idents[1].Value.(string); !ok || v != "SN-A1" {
		t.Errorf("Identifier[1]: got %T(%v) want string(SN-A1)", idents[1].Value, idents[1].Value)
	}
	if v, ok := idents[2].Value.(float64); !ok || v != 3.14 {
		t.Errorf("Identifier[2]: got %T(%v) want float64(3.14)", idents[2].Value, idents[2].Value)
	}
}

func TestReconstruct_NonStringLabelSilentlyDropped(t *testing.T) {
	// Label is string-typed in SensorML. A triple with a non-string Object
	// (operator bug) is silently dropped rather than coerced. This pins
	// that behavior so a future "be more permissive" change is explicit.
	triples := []message.Triple{
		{Subject: "drone.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
		{Subject: "drone.001", Predicate: labelPred, Object: 42}, // operator bug: int label
		{Subject: "drone.001", Predicate: descPred, Object: "A valid description"},
	}
	proc, err := reconstructProcessFromTriples(triples, "drone.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	if proc.Base().Label != "" {
		t.Errorf("Label: got %q want empty (non-string Object silently dropped)", proc.Base().Label)
	}
	if proc.Base().Description != "A valid description" {
		t.Errorf("Description: got %q (valid string should pass through)", proc.Base().Description)
	}
}

func TestReconstruct_MultiTypeClassifiesByFirstMatch(t *testing.T) {
	// SOSA/SSN permits multi-typed entities; the SensorML reverse mapping
	// is single-classification. First matching rdf:type triple wins.
	// Deterministic only modulo triple emission order — future-self
	// should not depend on this across framework versions.
	triples := []message.Triple{
		{Subject: "x.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
		{Subject: "x.001", Predicate: typePredCanon, Object: sosa.Sensor},
		{Subject: "x.001", Predicate: labelPred, Object: "Dual-typed thing"},
	}
	proc, err := reconstructProcessFromTriples(triples, "x.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	if _, ok := proc.(*sensorml.PhysicalSystem); !ok {
		t.Errorf("first rdf:type wins: got %T want *sensorml.PhysicalSystem", proc)
	}
}

func TestReconstruct_MinimalValidEntity(t *testing.T) {
	// The smallest legal System: rdf:type triple only. All three media
	// types must produce well-formed output. This is what Team Engine's
	// conformance suite is most likely to throw at us first.
	triples := []message.Triple{
		{Subject: "minimal.001", Predicate: typePredCanon, Object: sosa.SSNSystem},
	}
	proc, err := reconstructProcessFromTriples(triples, "minimal.001")
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	sys, ok := proc.(*sensorml.PhysicalSystem)
	if !ok {
		t.Fatalf("type: got %T", proc)
	}
	if sys.ID != "minimal.001" {
		t.Errorf("ID: got %q", sys.ID)
	}
	// All optional fields should be zero-valued, not nil-mishandled.
	if sys.Label != "" || sys.Description != "" || sys.Definition != "" {
		t.Errorf("optional fields not blank: %+v", sys.AbstractProcess)
	}
	if sys.AttachedTo != nil || len(sys.Components) != 0 {
		t.Errorf("composite fields not empty: AttachedTo=%v Components=%v", sys.AttachedTo, sys.Components)
	}
}

func TestSystemReconstructionFromState_RejectsEmpty(t *testing.T) {
	if _, err := systemReconstructionFromState(graph.EntityState{}); err == nil {
		t.Errorf("expected error for empty state, got nil")
	}
	if _, err := systemReconstructionFromState(graph.EntityState{ID: "acme.ops.robotics.gcs.system.empty"}); err == nil {
		t.Errorf("expected error for state with no triples, got nil")
	}
}
