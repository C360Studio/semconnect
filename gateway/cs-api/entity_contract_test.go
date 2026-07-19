package csapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	semtypes "github.com/c360studio/semstreams/pkg/types"
)

func TestMintEntityIDPreservesFittingIdentityAndBoundsOverflow(t *testing.T) {
	prefix := DefaultConfig().SystemIDPrefix
	if got, want := mintEntityID(prefix, []byte("urn:example:system:alpha")), prefix+".alpha"; got != want {
		t.Fatalf("fitting identity: got %q want %q", got, want)
	}

	got := mintEntityID(prefix, []byte(strings.Repeat("x", 300)))
	want := prefix + ".h-0d4e2ca9e9cbced7a7a5380eb29e1a3783b9b6d0db72de36a1051038e1c1fbc7"
	if got != want {
		t.Fatalf("overflow identity: got %q want %q", got, want)
	}
	if err := semtypes.ValidateEntityID(got); err != nil {
		t.Fatalf("overflow identity contract: %v", err)
	}
}

func TestLongSystemUIDRemainsInSemanticField(t *testing.T) {
	uid := strings.Repeat("x", 300)
	body, err := json.Marshal(map[string]any{
		"type": "Feature",
		"properties": map[string]any{
			"uid":  uid,
			"name": "Long-ID system",
		},
	})
	if err != nil {
		t.Fatalf("marshal feature: %v", err)
	}
	component := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})
	entityID, triples, err := component.buildSystemTriplesFromFeature(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromFeature(): %v", err)
	}
	wantID := component.cfg.SystemIDPrefix + ".h-0d4e2ca9e9cbced7a7a5380eb29e1a3783b9b6d0db72de36a1051038e1c1fbc7"
	if entityID != wantID {
		t.Fatalf("entity ID: got %q want %q", entityID, wantID)
	}
	gotUID, ok := firstStringObject(triples, PredSystemUID)
	if !ok || gotUID != uid {
		t.Fatalf("semantic UID: got %q present=%t, want exact source", gotUID, ok)
	}
}

func TestNestedAndArtifactMintingUseDomainSeparatedDigestSources(t *testing.T) {
	cfg := DefaultConfig()
	parent := cfg.SystemIDPrefix + ".parent"
	if got, want := mintNestedSensorMLEntityID(parent, "camera"), parent+"_camera"; got != want {
		t.Fatalf("fitting nested identity: got %q want %q", got, want)
	}

	gotChild := mintNestedSensorMLEntityID(parent, strings.Repeat("y", 300))
	wantChild := cfg.SystemIDPrefix + ".h-16756c6a7c74e4f2a441da07e9fdd45f8f986bb61a4c3340fa737e244c6592b4"
	if gotChild != wantChild {
		t.Fatalf("overflow child identity: got %q want %q", gotChild, wantChild)
	}

	maxParent := "a.b.c.d.e." + strings.Repeat("p", 246)
	gotArtifact := mintSchemaArtifactID(cfg.SchemaArtifactIDPrefix, maxParent, "resultSchema")
	wantArtifact := cfg.SchemaArtifactIDPrefix + ".h-604c6d1f09d9706579d1f70d8a82eb8c8e9f44e39f2e3d7f4b31d834183485c6"
	if gotArtifact != wantArtifact {
		t.Fatalf("overflow artifact identity: got %q want %q", gotArtifact, wantArtifact)
	}
	for _, id := range []string{gotChild, gotArtifact} {
		if err := semtypes.ValidateEntityID(id); err != nil {
			t.Errorf("minted identity %q: %v", id, err)
		}
	}
}

func TestConfigRejectsPrefixWithoutDigestBudget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SystemIDPrefix = "a.b.c.d." + strings.Repeat("e", 190)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "insufficient room") {
		t.Fatalf("Validate(): got %v, want digest-budget error", err)
	}
}

func TestAuthoritativeFinalStateValidationRejectsBeforeNATS(t *testing.T) {
	entityID := "acme.ops.robotics.gcs.system.alpha"
	tests := []struct {
		name   string
		triple message.Triple
	}{
		{
			name:   "invalid predicate",
			triple: message.Triple{Subject: entityID, Predicate: "cs-api.system.camelCase", Object: "bad"},
		},
		{
			name: "invalid entity reference",
			triple: message.Triple{
				Subject: entityID, Predicate: sensorml.PredIsHostedBy,
				Object: "not-an-entity-id", Datatype: message.EntityReferenceDatatype,
			},
		},
		{
			name: "non-string entity reference",
			triple: message.Triple{
				Subject: entityID, Predicate: sensorml.PredIsHostedBy,
				Object: 42, Datatype: message.EntityReferenceDatatype,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			component := newTestComponent(t, fake)
			err := component.ingestProjectedTriples(
				context.Background(), entityID, []message.Triple{tt.triple}, message.Type{}, Identity{},
			)
			if err == nil || !errs.IsInvalid(err) {
				t.Fatalf("ingestProjectedTriples(): got %v, want invalid error", err)
			}
			if fake.gotSubject != "" {
				t.Fatalf("NATS request occurred for invalid final state: %q", fake.gotSubject)
			}
		})
	}
}
