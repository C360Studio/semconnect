// Stage 20 — CS API §6 Procedure resource. Mirrors /systems read
// side: GET collection, GET item, OPTIONS, POST. Distinct from
// systems by rdf:type (sosa.Procedure vs sosa.SSNSystem) and by a
// spec-mandated lack of location — per OGC 23-001
// /req/procedure/location, Procedures MUST NOT carry geometry.
// The JSON shape omits `geometry` accordingly.
//
// **Why a separate file (vs. parameterized abstraction over
// /systems):** at Stage 20 we have only 2 resource types of this
// shape (Systems, Procedures). The user's CLAUDE.md "don't design
// for hypothetical future requirements; three similar lines is
// better than premature abstraction" applies. Stage 21+
// (Deployments, SamplingFeatures, Properties) is when the 4-way
// duplication justifies extraction; we'll evaluate then.
//
// **Endpoints landed at Stage 20:** GET/HEAD /procedures, GET/HEAD
// /procedures/{id}, POST /procedures, OPTIONS for both. PUT /
// DELETE / PATCH on /procedures are NOT landed here because the
// ETS CRD/Update test groups only target /systems — the existing
// conf/create-replace-delete + conf/update claims remain honest at
// /systems-only without expanding. If a real client (or a future
// ETS version) asks for procedure mutation, follow the Stage 16/19
// pattern and ship them as a follow-up.
package csapi

import (
	"encoding/json"
	"net/http"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// procedureCollection mirrors systemCollection. CS API §6 uses the
// `items` field name per OGC API Common §7.14 (same convention
// Stage 10 fixed for /systems).
type procedureCollection struct {
	Type           string         `json:"type"` // "ProcedureCollection"
	NumberMatched  int            `json:"numberMatched"`
	NumberReturned int            `json:"numberReturned"`
	Truncated      bool           `json:"truncated,omitempty"`
	Items          []procedureRef `json:"items"`
	Links          []link         `json:"links"`
}

type procedureRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "Procedure"
	Links []link `json:"links,omitempty"`
}

// procedure is the JSON shape returned by GET /procedures/{id}. No
// `geometry` field per /req/procedure/location — procedures don't
// carry location data (a procedure is a *method*, not a physical
// thing). Stage 18 uid preservation applies identically to systems.
type procedure struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"` // "Procedure"
	Label             string             `json:"label,omitempty"`
	Description       string             `json:"description,omitempty"`
	Definition        string             `json:"definition,omitempty"`
	UID               string             `json:"uid,omitempty"`
	UniqueID          string             `json:"uniqueId,omitempty"`
	FeatureProperties *featureProperties `json:"properties,omitempty"`
	Links             []link             `json:"links"`
}

// procedureFromState collapses an EntityState into the v0.1 JSON
// shape. Mirrors systemFromState but without the geometry path.
func procedureFromState(state graph.EntityState) procedure {
	p := procedure{
		ID:   state.ID,
		Type: "Procedure",
		Links: []link{
			{Href: "/procedures/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/procedures/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		p.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		p.Description = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDefinition); ok {
		p.Definition = v
	}
	if v, ok := firstStringObject(state.Triples, PredSystemUID); ok {
		p.UID = v
		p.UniqueID = v
		p.FeatureProperties = &featureProperties{
			UID:         v,
			Name:        p.Label,
			Description: p.Description,
		}
	}
	return p
}

// isProcedureKind reports whether the entity's rdf:type is
// sosa.Procedure. Symmetric with isSystemKind / isDatastreamKind.
func isProcedureKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	return typeIRI == sosa.Procedure
}

// handleProcedures serves GET /procedures.
func (c *Component) handleProcedures(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), sosa.Procedure, limit, "listProcedureEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	coll := procedureCollection{
		Type:           "ProcedureCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Items:          make([]procedureRef, 0, len(ids)),
		Links: []link{
			{Href: "/procedures", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range ids {
		coll.Items = append(coll.Items, procedureRef{
			ID:   id,
			Type: "Procedure",
			Links: []link{
				{Href: "/procedures/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/procedures/" + id, Rel: "canonical", Type: string(MediaJSON)},
			},
		})
	}
	if len(ids) >= limit {
		coll.Truncated = true
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

// handleProcedure serves GET /procedures/{id}.
func (c *Component) handleProcedure(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}

	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid procedure id: "+err.Error())
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	if !isProcedureKind(state.Triples) {
		c.logger.Info("entity not a procedure kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "entity is not a Procedure")
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(procedureFromState(state))
}

// handleProceduresOptions serves OPTIONS /procedures. Read-only +
// POST at Stage 20; CRD verbs (PUT/DELETE) excluded because the ETS
// CRD test group only targets /systems.
func (c *Component) handleProceduresOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// handleProcedureOptions serves OPTIONS /procedures/{id}.
func (c *Component) handleProcedureOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid procedure id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
