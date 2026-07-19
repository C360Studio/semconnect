package csapi

import (
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/ownership"
	"github.com/c360studio/semstreams/pkg/projection"
)

func TestSystemProjectionContract_DerivesNoBirthStubForeignEdge(t *testing.T) {
	cfg := DefaultConfig()
	contract := systemProjectionContract(cfg.SystemIDPrefix)

	reg, err := projection.Derive(systemProjectionOwner, contract)
	if err != nil {
		t.Fatalf("derive System projection contract: %v", err)
	}
	if reg.Owner != systemProjectionOwner {
		t.Fatalf("owner: got %q want %q", reg.Owner, systemProjectionOwner)
	}
	if len(reg.Claims) != 0 {
		t.Fatalf("System foreign-edge contract should not claim owned predicate groups yet: %+v", reg.Claims)
	}
	if len(reg.ForeignEdges) != 1 {
		t.Fatalf("foreign edges: got %d want 1 (%+v)", len(reg.ForeignEdges), reg.ForeignEdges)
	}

	edge := reg.ForeignEdges[0]
	if edge.Producer != systemProjectionMessageType.Key() {
		t.Errorf("producer: got %q want %q", edge.Producer, systemProjectionMessageType.Key())
	}
	if edge.Predicate != sensorml.PredIsHostedBy {
		t.Errorf("predicate: got %q want %q", edge.Predicate, sensorml.PredIsHostedBy)
	}
	if edge.Mode != ownership.EdgeNoBirthStub {
		t.Errorf("mode: got %q want %q", edge.Mode, ownership.EdgeNoBirthStub)
	}
	if want := cfg.SystemIDPrefix + ".*"; edge.TargetPattern != want {
		t.Errorf("target pattern: got %q want %q", edge.TargetPattern, want)
	}
}
