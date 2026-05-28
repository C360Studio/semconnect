package csapi

import (
	"errors"
	"fmt"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// Reverse mapping: graph.EntityState triples → sensorml.Process.
//
// The forward direction (sensorml.Asset.Triples()) flattens a SensorML
// document into RDF-style triples; this file inverts the subset of that
// mapping that CS API §7.2 needs to render a System resource. The
// reconstruction is intentionally **lossy** — Asset.Triples never emitted
// the following, so this file cannot recover them:
//
//   - Inputs / Outputs / Parameters (DataComponentList) — graph layer
//     doesn't carry SWE Common typed slots yet
//   - Keywords []string
//   - TypeOf reference
//   - Connections list (PhysicalSystem/AggregateProcess intra-component wiring)
//   - Identifier.Definition / Label / UoM (only Value survives)
//   - Capability / Characteristic metadata (only Value survives)
//
// Stage 4 v0.1 documents this limitation in the X-CS-Reconstructed-Lossy
// response header. Operators who need lossless round-trip can fetch the
// original SensorML JSON via the StorageRef field of EntityState (deferred
// to a follow-up — graph-ingest's storage seam exists but is not wired
// through cs-api yet).
//
// Non-string predicate objects: triples carry Object as `any`. For string-
// typed SensorML fields (label, description, IRIs in usedProcedure /
// attachedTo / hosts / rdf:type), a non-string object is silently dropped
// by firstStringObject / allStringObjects. This is the right behavior for
// label-shaped fields where coercion would corrupt the semantics — an int
// label would round-trip as "42" if we coerced, losing the type signal.
// Identifier.Value, Capability.Value, and Characteristic.Value preserve the
// any-typed object exactly (SensorML and OMS both allow numeric values
// there). Operators who emit non-string labels or descriptions get
// reconstructed entities with those fields blank; the JSON-LD path still
// shows the raw triple via vocabulary/export.
//
// Multi-rdf:type entities: SOSA/SSN allows an entity to assert multiple
// rdf:type classifications (a System that is also a Sensor). The reverse
// mapping is single-classification by SensorML design — Process is a
// single concrete kind. We log a warn when multiple type triples are
// observed and pick the first matching one. The chosen kind is
// deterministic only modulo the triple emission order; future-self should
// not rely on order across framework versions.
//
// Predicate aliasing: triples emitted via sensorml.Asset.Triples() use the
// dotted form (e.g. "sensorml.process.type"); triples emitted by raw
// producers may use the canonical "rdf.type" form. Both are accepted as
// the rdf:type predicate. Other predicates (label, description, etc.) are
// only accepted in their sensorml.process.* dotted form — operators who
// want round-trip support emit via sensorml.Asset.

// typeAliases lists the predicate names that mean "rdf:type". The first form
// is what sensorml.Asset emits today; the second is the bare canonical form.
// When the framework switches to IRI-keyed predicates, this set shrinks.
var typeAliases = []string{sensorml.PredType, "rdf.type"}

// reconstructProcessFromTriples builds a sensorml.Process from the entity
// state. entityID is the 6-part SemStreams ID — it becomes the AbstractProcess
// .ID field of the reconstructed Process so subsequent forward marshalling
// preserves the address.
//
// Returns an error when no rdf:type triple is found (the entity is not a
// SensorML-describable thing) or when the type IRI is not one of the four
// CS API critical-path kinds (ADR-044 Phase 5 scope cut).
func reconstructProcessFromTriples(triples []message.Triple, entityID string) (sensorml.Process, error) {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return nil, errors.New("entity has no rdf:type triple")
	}

	base := buildAbstractProcess(triples, entityID)

	switch typeIRI {
	case sosa.SSNSystem:
		sys := &sensorml.PhysicalSystem{AbstractProcess: base}
		sys.Components = reconstructHostedComponents(triples)
		if attached, ok := firstStringObject(triples, sensorml.PredAttachedTo); ok {
			sys.AttachedTo = &sensorml.Reference{Href: attached}
		}
		return sys, nil

	case sosa.Sensor:
		comp := &sensorml.PhysicalComponent{AbstractProcess: base}
		if proc, ok := firstStringObject(triples, sensorml.PredUsedProcedure); ok {
			comp.Method = &sensorml.Reference{Href: proc}
		}
		if attached, ok := firstStringObject(triples, sensorml.PredAttachedTo); ok {
			comp.AttachedTo = &sensorml.Reference{Href: attached}
		}
		return comp, nil

	case sosa.Procedure:
		// sosa:Procedure maps to either SimpleProcess (leaf) or
		// AggregateProcess (has children via ssn:hasSubSystem).
		// Disambiguate by presence of subsystem triples — same
		// discriminator the forward Asset.Triples uses.
		subs := allStringObjects(triples, sensorml.PredHasSubSystem)
		if len(subs) > 0 {
			agg := &sensorml.AggregateProcess{AbstractProcess: base}
			agg.Components = childRefsFromIDs(subs, sensorml.TypeSimpleProcess)
			return agg, nil
		}
		sp := &sensorml.SimpleProcess{AbstractProcess: base}
		if proc, ok := firstStringObject(triples, sensorml.PredUsedProcedure); ok {
			sp.Method = &sensorml.Reference{Href: proc}
		}
		return sp, nil

	default:
		return nil, fmt.Errorf("unsupported rdf:type IRI %q (cs-api v0.1 supports %s, %s, %s)",
			typeIRI, sosa.SSNSystem, sosa.Sensor, sosa.Procedure)
	}
}

// buildAbstractProcess populates the fields shared by every concrete process
// kind from the triples. Lossy: see file doc comment.
func buildAbstractProcess(triples []message.Triple, entityID string) sensorml.AbstractProcess {
	ap := sensorml.AbstractProcess{ID: entityID}
	// Preserve the client-submitted uniqueId from the framework uid
	// triple. Absent triple = absent UniqueID on the response (no
	// synthetic fallback).
	if v, ok := firstSystemUIDObject(triples); ok {
		ap.UniqueID = v
	}
	if v, ok := firstStringObject(triples, sensorml.PredLabel); ok {
		ap.Label = v
	}
	if v, ok := firstStringObject(triples, sensorml.PredDescription); ok {
		ap.Description = v
	}
	if v, ok := firstStringObject(triples, sensorml.PredDefinition); ok {
		ap.Definition = v
	}
	for _, t := range triples {
		if t.Predicate == sensorml.PredIdentifierValue {
			ap.Identifiers = append(ap.Identifiers, sensorml.Term{Value: t.Object})
		}
	}
	for _, t := range triples {
		switch t.Predicate {
		case sensorml.PredCapabilityValue:
			ap.Capabilities = append(ap.Capabilities, sensorml.Term{Value: t.Object})
		case sensorml.PredCharacteristicValue:
			ap.Characteristics = append(ap.Characteristics, sensorml.Term{Value: t.Object})
		}
	}
	return ap
}

// reconstructHostedComponents builds skeleton PhysicalComponent entries for
// every ssn:hosts triple. Skeleton because we do NOT recurse via NATS to
// hydrate each child — clients drill down via GET /systems/{childID}. CS API
// §7.13 allows the reference shape; embedding is a post-v0.1 ?embed=true.
func reconstructHostedComponents(triples []message.Triple) []sensorml.Process {
	ids := allStringObjects(triples, sensorml.PredHosts)
	if len(ids) == 0 {
		return nil
	}
	return childRefsFromIDs(ids, sensorml.TypePhysicalComponent)
}

// childRefsFromIDs emits minimal Process entries — ID + Type discriminator
// only, no semantic fields. The framework's SensorML JSON encoder emits
// these as valid (if sparse) Process records; CS API clients re-fetch each
// child via its own GET to hydrate.
func childRefsFromIDs(ids []string, kind string) []sensorml.Process {
	out := make([]sensorml.Process, 0, len(ids))
	for _, id := range ids {
		base := sensorml.AbstractProcess{ID: id}
		switch kind {
		case sensorml.TypePhysicalComponent:
			out = append(out, &sensorml.PhysicalComponent{AbstractProcess: base})
		case sensorml.TypeSimpleProcess:
			out = append(out, &sensorml.SimpleProcess{AbstractProcess: base})
		}
	}
	return out
}

// firstStringObject returns the Object of the first triple whose predicate
// matches any of preds, type-asserted to string. ok=false when no triple
// matches or the matching triple's object is not a string.
func firstStringObject(triples []message.Triple, preds ...string) (string, bool) {
	for _, t := range triples {
		for _, p := range preds {
			if t.Predicate != p {
				continue
			}
			if s, ok := t.Object.(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

// allStringObjects returns every string Object across triples whose predicate
// equals pred. Order is preserved; non-string objects are skipped.
func allStringObjects(triples []message.Triple, pred string) []string {
	var out []string
	for _, t := range triples {
		if t.Predicate != pred {
			continue
		}
		if s, ok := t.Object.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// systemReconstructionFromState wraps reconstructProcessFromTriples with a
// state-shaped input. Pulled out so the handler doesn't have to thread
// triples + entityID separately.
func systemReconstructionFromState(state graph.EntityState) (sensorml.Process, error) {
	if state.ID == "" {
		return nil, errors.New("entity state missing ID")
	}
	if len(state.Triples) == 0 {
		return nil, errors.New("entity state has no triples")
	}
	return reconstructProcessFromTriples(state.Triples, state.ID)
}
