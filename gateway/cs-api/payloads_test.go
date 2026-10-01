package csapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/vocabulary"
)

func TestRegisteredProjectionPayloadsRoundTrip(t *testing.T) {
	registry := payloadregistry.New()
	if err := RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	contracts := registry.Contracts()
	if len(contracts) != 11 || len(registry.List()) != 11 {
		t.Fatalf("missing product types/contracts: %d/%d", len(registry.List()), len(contracts))
	}
	for _, contract := range contracts {
		t.Run(contract.Name, func(t *testing.T) {
			mt := contract.MessageType
			payload, ok := registry.Create(mt.Domain, mt.Category, mt.Version).(*ProjectedResource)
			if !ok {
				t.Fatal("factory did not return product-owned ProjectedResource")
			}
			id := strings.ReplaceAll(contract.EntityPattern, "*", "reference")
			payload.ID = id
			payload.Facts = []message.Triple{{Subject: id, Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"}}
			if contract.Name == schemaArtifactProjectionContractName {
				payload.Storage = &message.StorageReference{StorageInstance: "objectstore", Key: "reference.json", ContentType: "application/json", Size: 3}
			}
			encoded, err := json.Marshal(message.NewBaseMessage(mt, payload, "setup03a"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := message.NewDecoder(registry).Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := decoded.Payload().(*ProjectedResource)
			if !ok || got.Schema() != mt || got.EntityID() != id || !reflect.DeepEqual(got.Triples(), payload.Facts) || !storageReferencesEqual(got.StorageRef(), payload.Storage) {
				t.Fatalf("decoder lost concrete payload contract: %#v", decoded.Payload())
			}
			if profile, registered := registry.IndexingProfileFor(mt.Key()); !registered || profile != vocabulary.IndexingProfileContent {
				t.Fatalf("registered floor: %s %v", profile, registered)
			}
			var graphable graph.Graphable = got
			if graphable.EntityID() != id {
				t.Fatal("graphable identity changed")
			}
			got.Facts[0].Subject = "acme.other.systems.csapi.system.foreign"
			if _, err := json.Marshal(message.NewBaseMessage(mt, got, "setup03a")); err == nil {
				t.Fatal("publication accepted foreign-subject facts")
			}
		})
	}
}

func TestRegisteredProjectionPayloadsPreserveConfiguredPrefixes(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SystemIDPrefix = "acme.remote.assets.csapi.system"
	registry := payloadregistry.New()
	if err := RegisterPayloadsWithConfig(registry, cfg); err != nil {
		t.Fatal(err)
	}
	mt := systemProjectionMessageType
	payload := registry.Create(mt.Domain, mt.Category, mt.Version).(*ProjectedResource)
	payload.ID = cfg.SystemIDPrefix + ".one"
	payload.Facts = []message.Triple{{Subject: payload.ID, Predicate: sensorml.PredType, Object: "http://www.w3.org/ns/ssn/System"}}
	if err := payload.Validate(); err != nil {
		t.Fatal(err)
	}
	payload.ID = DefaultConfig().SystemIDPrefix + ".one"
	payload.Facts[0].Subject = payload.ID
	if err := payload.Validate(); err == nil {
		t.Fatal("payload escaped configured prefix")
	}
	if err := RegisterPayloads(nil); err == nil {
		t.Fatal("nil registry accepted")
	}
}

func TestProjectedResourceRejectsMissingAuthorityAndUndeclaredFacts(t *testing.T) {
	var absent *ProjectedResource
	if err := absent.Validate(); err == nil {
		t.Fatal("nil resource accepted")
	}
	if _, err := json.Marshal(&ProjectedResource{}); err == nil {
		t.Fatal("unbound resource accepted for publication")
	}
	registry := payloadregistry.New()
	if err := RegisterPayloads(registry); err != nil {
		t.Fatal(err)
	}
	mt := systemProjectionMessageType
	payload := registry.Create(mt.Domain, mt.Category, mt.Version).(*ProjectedResource)
	payload.ID = DefaultConfig().SystemIDPrefix + ".invalid"
	payload.Facts = []message.Triple{{Subject: payload.ID, Predicate: "consumer.undeclared.fact", Object: "unowned"}}
	if _, err := json.Marshal(payload); err == nil {
		t.Fatal("unowned predicate accepted for publication")
	}
	cfg := DefaultConfig()
	cfg.SystemIDPrefix = "invalid-prefix"
	if err := RegisterPayloadsWithConfig(payloadregistry.New(), cfg); err == nil {
		t.Fatal("invalid configured authority accepted")
	}
}
