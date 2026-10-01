package csapi

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/payloadregistry"
	"github.com/c360studio/semstreams/pkg/projection/contract"
)

// ProjectedResource is the product-owned graph projection payload. Its factory
// binds the resource contract; wire data cannot select a different schema or
// expand the permitted subjects and predicates. HTTP representations remain
// owned by their endpoint-specific builders.
type ProjectedResource struct {
	ID      string                    `json:"id"`
	Facts   []message.Triple          `json:"triples"`
	Storage *message.StorageReference `json:"storage_ref,omitempty"`

	contract contract.Contract
}

func (p *ProjectedResource) Schema() message.Type                  { return p.contract.MessageType }
func (p *ProjectedResource) EntityID() string                      { return p.ID }
func (p *ProjectedResource) Triples() []message.Triple             { return p.Facts }
func (p *ProjectedResource) StorageRef() *message.StorageReference { return p.Storage }

func (p *ProjectedResource) Validate() error {
	if p == nil {
		return errors.New("nil projected resource")
	}
	if err := p.contract.Validate(); err != nil {
		return fmt.Errorf("projected resource contract: %w", err)
	}
	if err := validateProjectedTriples(p.ID, p.Facts); err != nil {
		return err
	}
	if err := validateProjectionFacts(p.contract, p.ID, p.Facts); err != nil {
		return err
	}
	return graph.ValidateEntityStateContract(&graph.EntityState{
		ID: p.ID, Triples: p.Facts, MessageType: p.Schema(), StorageRef: p.Storage,
	})
}

func (p *ProjectedResource) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	type wire ProjectedResource
	return json.Marshal((*wire)(p))
}

func (p *ProjectedResource) UnmarshalJSON(data []byte) error {
	type wire ProjectedResource
	return json.Unmarshal(data, (*wire)(p))
}

// RegisterPayloads installs SemConnect's eleven resource types into the host's
// single type authority. The host calls this after payloadbuiltins.Register.
func RegisterPayloads(reg *payloadregistry.Registry) error {
	return RegisterPayloadsWithConfig(reg, DefaultConfig())
}

// RegisterPayloadsWithConfig preserves the operator's configured projection
// prefixes when embedding SemConnect in a custom graph host.
func RegisterPayloadsWithConfig(reg *payloadregistry.Registry, cfg Config) error {
	if reg == nil {
		return errors.New("cs-api: payload registry required")
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("cs-api: payload config: %w", err)
	}
	for _, projection := range projectionContracts(cfg) {
		mt := projection.MessageType
		if err := reg.Register(&payloadregistry.Registration{
			Domain: mt.Domain, Category: mt.Category, Version: mt.Version,
			Description:     "SemConnect " + projection.Name + " graph projection",
			Factory:         func() any { return &ProjectedResource{contract: projection} },
			IndexingProfile: projection.IndexingProfile,
			Contracts:       []contract.Contract{projection},
		}); err != nil {
			return fmt.Errorf("register %s: %w", mt.Key(), err)
		}
	}
	return nil
}
