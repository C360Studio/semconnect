package csapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
)

// TestForeignEdgeBakeFixture_EmitsForeignIsHostedByEdge pins the conformance
// harness's foreign-edge bake fixture against drift.
//
// The ADR-055/056 must-exist-flip readiness bake (conformance/run.sh) asserts
// graph-ingest's foreign_edge_unclaimed_total reads zero. That assertion only
// proves "zero by claimed" — rather than the meaningless "zero by absence" — if
// the seed actually emits a foreign-subject `child isHostedBy parent` edge. That
// edge originates from a SensorML PhysicalSystem with an inline component, so if
// conformance/fixtures/system-hosted.sml.json ever loses its `components` block
// (or the parser stops emitting the foreign edge), the bake silently degrades to
// proving nothing. This test fails loudly at that point.
//
// It mirrors the assertion in TestHandleSystemPost_SensorMLComponentForeignEdgeForwarded,
// but reads the on-disk conformance fixture so the file the harness POSTs is the
// thing under test.
func TestForeignEdgeBakeFixture_EmitsForeignIsHostedByEdge(t *testing.T) {
	fixture := filepath.Join("..", "..", "conformance", "fixtures", "system-hosted.sml.json")
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read bake fixture %s: %v", fixture, err)
	}

	// Typed-nil requester: New's nil-check sees a non-nil interface value, so
	// construction succeeds, and buildSystemTriplesFromSensorML only mints an ID
	// + calls asset.Triples() — it never dials the requester.
	c := newTestComponent(t, nil)
	entityID, triples, err := c.buildSystemTriplesFromSensorML(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromSensorML(%s): %v", fixture, err)
	}
	if entityID == "" {
		t.Fatal("fixture produced an empty parent entity ID")
	}

	var sawForeignEdge bool
	for _, tr := range triples {
		if tr.Predicate != sensorml.PredIsHostedBy {
			continue
		}
		// Foreign-subject edge: the subject (the hosted child) is NOT the
		// entity being created (the parent). That is what makes graph-ingest
		// route it through the ForeignEdgeClaim seam under the must-exist flip.
		if tr.Subject == entityID {
			t.Errorf("isHostedBy edge has subject==entityID %q — that is an own-subject edge, not the foreign edge the bake needs", entityID)
			continue
		}
		if tr.Object != entityID {
			t.Errorf("isHostedBy edge object=%q want parent entityID %q", tr.Object, entityID)
		}
		sawForeignEdge = true
	}
	if !sawForeignEdge {
		t.Fatalf("bake fixture emitted no foreign isHostedBy edge — the conformance bake would read zero-by-absence, proving nothing. Triples: %+v", triples)
	}
}
