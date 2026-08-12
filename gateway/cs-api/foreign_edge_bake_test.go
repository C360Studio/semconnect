package csapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
)

func TestHostedFixtureProjectsOnlyRootSubjectFacts(t *testing.T) {
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

	var sawRootHost bool
	for _, tr := range triples {
		if tr.Subject != entityID {
			t.Fatalf("foreign-subject fact escaped root filter: %+v", tr)
		}
		sawRootHost = sawRootHost || tr.Predicate == sensorml.PredHosts
	}
	if !sawRootHost {
		t.Fatalf("fixture lost root-to-child relationship: %+v", triples)
	}
}
