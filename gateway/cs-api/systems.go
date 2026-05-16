package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/vocabulary/export"
	"github.com/c360studio/semstreams/vocabulary/sosa"
	"github.com/nats-io/nats.go"
)

// CS API resource shape for the System collection. v0.1 returns just entity
// IDs — Stage 4 (single-system GET with SensorML round-trip) is where we
// reconstruct full uniqueId / label / location / capabilities properties from
// the entity-state triples.
type systemRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "System" per CS API §7.2 nominal class
	Links []link `json:"links,omitempty"`
}

type link struct {
	Href  string `json:"href"`
	Rel   string `json:"rel"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

type systemCollection struct {
	Type string `json:"type"` // "SystemCollection"

	// NumberMatched is the total number of entities in the graph that match
	// the query, independent of paging. CS API §7.13 expects this distinct
	// from NumberReturned. graph-index's predicate-query subject does not
	// return a total — for v0.1 we set this equal to NumberReturned and
	// flip on Truncated when the page filled to the requested limit. A
	// future predicate-query enhancement (or a separate count subject) will
	// retire the lie. Track as a Stage 4+ follow-up.
	NumberMatched  int         `json:"numberMatched"`
	NumberReturned int         `json:"numberReturned"`
	Truncated      bool        `json:"truncated,omitempty"`
	Systems        []systemRef `json:"systems"`
	Links          []link      `json:"links"`
}

// NATS subjects + the dotted predicate name the framework's payload registry
// registers `rdf:type` under (vocabulary/README.md example). Object value is
// the full SSN System IRI.
const (
	subjectPredicateQuery = "graph.index.query.predicate"
	subjectEntityQuery    = "graph.query.entity"
	predicateRDFType      = "rdf.type"
)

// system is the JSON shape returned by GET /systems/{id}. CS API §7.2's
// System resource has many more fields; v0.1 surfaces what the reverse
// mapping can populate without recursing into child entities. Lossy fields
// (inputs/outputs, keywords, connections, identifier metadata) are
// documented in gateway/cs-api/sensorml.go. Lossy-reconstruction signalling
// lives on the X-CS-Reconstructed-Lossy response header — single source so
// header and body cannot drift.
type system struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"` // "System"
	Label         string   `json:"label,omitempty"`
	Description   string   `json:"description,omitempty"`
	Definition    string   `json:"definition,omitempty"`
	Hosts         []string `json:"hosts,omitempty"`
	HostedBy      string   `json:"hostedBy,omitempty"`
	UsedProcedure string   `json:"usedProcedure,omitempty"`
	AttachedTo    string   `json:"attachedTo,omitempty"`
	Identifiers   []any    `json:"identifiers,omitempty"`
	Capabilities  []any    `json:"capabilities,omitempty"`
	Properties    []any    `json:"properties,omitempty"`
	Links         []link   `json:"links"`
}

// systemFromState collapses an EntityState into the v0.1 JSON shape. Mirrors
// what reconstructProcessFromTriples does, but for the JSON media type
// rather than the SensorML one — both read the same predicate set.
func systemFromState(state graph.EntityState) system {
	s := system{
		ID:   state.ID,
		Type: "System",
		Links: []link{
			{Href: "/systems/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/systems/" + state.ID, Rel: "alternate", Type: string(MediaSensorML)},
			{Href: "/systems/" + state.ID, Rel: "alternate", Type: string(MediaJSONLD)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		s.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		s.Description = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDefinition); ok {
		s.Definition = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredUsedProcedure); ok {
		s.UsedProcedure = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredAttachedTo); ok {
		s.AttachedTo = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredIsHostedBy); ok {
		s.HostedBy = v
	}
	s.Hosts = allStringObjects(state.Triples, sensorml.PredHosts)
	for _, t := range state.Triples {
		switch t.Predicate {
		case sensorml.PredIdentifierValue:
			s.Identifiers = append(s.Identifiers, t.Object)
		case sensorml.PredCapabilityValue:
			s.Capabilities = append(s.Capabilities, t.Object)
		case sensorml.PredCharacteristicValue:
			s.Properties = append(s.Properties, t.Object)
		}
	}
	return s
}

// handleSystems serves GET /systems. CS API §7.13.
//
// Flow:
//  1. Negotiate Accept (Stage 2 wires JSON only — 406 for everything else;
//     SensorML / JSON-LD encoders land at Stage 4 and widen the supported set).
//  2. Parse ?limit= against the configured ceiling.
//  3. NATS request to graph.index.query.predicate filtering rdf:type = ssn:System.
//  4. Shape into CS API SystemCollection JSON.
func (c *Component) handleSystems(w http.ResponseWriter, r *http.Request) {
	// Method is enforced by the ServeMux pattern ("GET /systems",
	// "HEAD /systems"); non-matching methods 405 before reaching here.
	// FamilySystemCollection's supported() is JSON-only, so a SensorML or
	// JSON-LD Accept honestly 406s with an advertised set that matches
	// what we can actually emit (no SystemCollection wrapper exists).
	if _, ok := Negotiate(r.Header.Get("Accept"), FamilySystemCollection); !ok {
		WriteNotAcceptable(w, FamilySystemCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	entities, err := c.listSystemEntities(r.Context(), limit)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	coll := systemCollection{
		Type:           "SystemCollection",
		NumberMatched:  len(entities),
		NumberReturned: len(entities),
		Truncated:      len(entities) == limit, // see NumberMatched doc comment
		Systems:        make([]systemRef, 0, len(entities)),
		Links: []link{
			{Href: "/systems", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range entities {
		coll.Systems = append(coll.Systems, systemRef{
			ID:   id,
			Type: "System",
			Links: []link{
				{Href: "/systems/" + id, Rel: "self", Type: string(MediaJSON)},
			},
		})
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if encErr := json.NewEncoder(w).Encode(coll); encErr != nil {
		c.errs.Add(1)
		c.logger.Error("encode systems response", "err", encErr)
	}
}

// handleSystem serves GET /systems/{id}. CS API §7.2 (System resource).
//
// Flow:
//  1. Path-validate the entity ID (non-empty, NATS-token-safe — same rules
//     as datastream IDs since SemStreams 6-part IDs are NATS-shape).
//  2. Negotiate Accept across JSON / SensorML+JSON / JSON-LD.
//  3. NATS request to graph.query.entity to fetch the EntityState.
//  4. Detect "not found" via classifyEntityQueryError (until upstream
//     ships structured request-reply errors — issue filed with
//     C360Studio/semstreams).
//  5. Encode per the chosen media type:
//     - JSON:           shape from systemFromState (CS API §7.2 subset)
//     - SensorML+JSON:  reconstructProcessFromTriples → json.Marshal
//     - JSON-LD:        export.Serialize(triples, export.JSONLD)
func (c *Component) handleSystem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}

	media, ok := Negotiate(r.Header.Get("Accept"), FamilySystemItem)
	if !ok {
		WriteNotAcceptable(w, FamilySystemItem)
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	// Consistent gate across all three media types: if the entity is not
	// a representable System kind, every encoder 404s — JSON path no
	// longer emits a degraded body, JSON-LD no longer elides @context
	// silently, SensorML no longer 406s while the other two succeed.
	if !isSystemKind(state.Triples) {
		c.logger.Info("entity not a system kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "no system: "+id)
		return
	}

	// All three media types share the same Content-Type-setting header
	// dance; only the body encoding differs.
	switch media {
	case MediaJSON:
		c.writeSystemJSON(w, r, state)
	case MediaSensorML:
		c.writeSystemSensorML(w, r, state)
	case MediaJSONLD:
		c.writeSystemJSONLD(w, r, state)
	default:
		// Negotiate returned a media we didn't wire — defensive 406.
		WriteNotAcceptable(w, FamilySystemItem)
	}
}

// isSystemKind reports whether the entity's rdf:type maps to one of the
// SOSA/SSN classes /systems/{id} serves: ssn:System (PhysicalSystem),
// sosa:Sensor (PhysicalComponent in CS API parlance — Sensors are Systems),
// or sosa:Procedure (SimpleProcess / AggregateProcess). An entity of any
// other rdf:type (Observation, FeatureOfInterest, Deployment, …) is not a
// System and the URL space owes a 404.
func isSystemKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	switch typeIRI {
	case sosa.SSNSystem, sosa.Sensor, sosa.Procedure:
		return true
	}
	return false
}

// writeSystemJSON emits the v0.1 CS API §7.2 JSON shape (subset of full
// System resource — populated from triples).
func (c *Component) writeSystemJSON(w http.ResponseWriter, r *http.Request, state graph.EntityState) {
	sys := systemFromState(state)
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true") // see sensorml.go file doc
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(sys); err != nil {
		c.errs.Add(1)
		c.logger.Error("encode system JSON response", "id", state.ID, "err", err)
	}
}

// writeSystemSensorML emits application/sensorml+json via the reverse
// mapping in sensorml.go. The entity has already passed the isSystemKind
// gate in handleSystem, so any reconstruction failure here is a server-
// side data-integrity issue (a malformed triple set the gate didn't catch),
// not a client problem — 500.
func (c *Component) writeSystemSensorML(w http.ResponseWriter, r *http.Request, state graph.EntityState) {
	proc, err := systemReconstructionFromState(state)
	if err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "writeSystemSensorML", "reverse mapping"))
		return
	}
	body, err := json.Marshal(proc)
	if err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "writeSystemSensorML", "marshal sensorml"))
		return
	}
	w.Header().Set("Content-Type", string(MediaSensorML))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write SensorML response", "id", state.ID, "err", err)
	}
}

// writeSystemJSONLD emits application/ld+json via the framework's RDF
// emitter. Triples already compact to sosa:/ssn:/skos: prefixes thanks to
// vocabulary.Register calls in sensorml.predicates.init() and friends.
func (c *Component) writeSystemJSONLD(w http.ResponseWriter, r *http.Request, state graph.EntityState) {
	var buf bytes.Buffer
	if err := export.Serialize(&buf, state.Triples, export.JSONLD); err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "writeSystemJSONLD", "serialize jsonld"))
		return
	}
	w.Header().Set("Content-Type", string(MediaJSONLD))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := buf.WriteTo(w); err != nil {
		c.errs.Add(1)
		c.logger.Error("write JSON-LD response", "id", state.ID, "err", err)
	}
}

// fetchEntity issues the graph.query.entity request and parses the response.
// Classifies "not found" as Invalid (→ 404) so writeBackendError handles it
// uniformly with other input-side failures. Other NATS sentinels follow the
// Stage-2/3 pattern: ErrNoResponders / timeouts → Transient → 503.
func (c *Component) fetchEntity(ctx context.Context, id string) (graph.EntityState, error) {
	reqBody, err := json.Marshal(struct {
		ID string `json:"id"`
	}{ID: id})
	if err != nil {
		return graph.EntityState{}, errs.Wrap(err, "cs-api", "fetchEntity", "marshal entity query")
	}

	respBytes, err := c.nats.Request(ctx, subjectEntityQuery, reqBody, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return graph.EntityState{}, errs.WrapTransient(err, "cs-api", "fetchEntity", "graph backend unavailable")
		default:
			return graph.EntityState{}, errs.Wrap(err, "cs-api", "fetchEntity", "entity query")
		}
	}

	// Detect framework error-reply prefix (see classifyEntityQueryError).
	if classified := classifyEntityQueryError(respBytes); classified != nil {
		return graph.EntityState{}, classified
	}

	var state graph.EntityState
	if err := json.Unmarshal(respBytes, &state); err != nil {
		return graph.EntityState{}, errs.Wrap(err, "cs-api", "fetchEntity", "decode entity state")
	}
	return state, nil
}

// errEntityNotFound is the sentinel writeBackendError translates to 404.
// pkg/errs has no NotFound class today (Invalid / Transient / Fatal only);
// rather than overload Invalid → 400 to also mean "missing entity → 404",
// we keep a local sentinel here and have writeBackendError detect it. When
// upstream ships structured request-reply errors (see classifyEntityQueryError
// TODO) this can fold back into a framework class.
var errEntityNotFound = errors.New("cs-api: entity not found")

// classifyEntityQueryError parses the framework's unstructured error-reply
// format into a pkg/errs-classified error. Upstream issue filed with
// C360Studio/semstreams to ship structured NATS-header error responses; when
// that lands, this function becomes a no-op and the call site swaps to
// reading the X-Status header.
//
// Today natsclient/request.go replies with literal bytes `"error: " +
// err.Error()` on handler failure. graph-ingest produces these error tails:
//
//   - "error: not found: <id>"            → 404 (errEntityNotFound sentinel)
//   - "error: invalid request: ..."       → 400 (Invalid)
//   - "error: internal error: ..."        → 500 (unclassified)
//
// Returns nil when respBytes is a successful payload (no leading "error: ").
func classifyEntityQueryError(respBytes []byte) error {
	const prefix = "error: "
	if !bytes.HasPrefix(respBytes, []byte(prefix)) {
		return nil
	}
	tail := string(respBytes[len(prefix):])
	switch {
	case strings.HasPrefix(tail, "not found:"):
		return fmt.Errorf("%w: %s", errEntityNotFound, tail)
	case strings.HasPrefix(tail, "invalid request:"):
		return errs.WrapInvalid(errors.New(tail), "cs-api", "fetchEntity", "bad query")
	default:
		// Includes "internal error: ..." and any unknown tail.
		return errs.Wrap(errors.New(tail), "cs-api", "fetchEntity", "backend error")
	}
}

// listSystemEntities issues a NATS request to the predicate index for every
// entity whose rdf:type is ssn:System. Returns IDs only; full entity hydration
// lands when Stage 4 ships single-system GET.
//
// Errors are classified at this boundary so writeBackendError downstream can
// map cleanly to HTTP status. natsclient returns raw nats sentinels (it does
// NOT wrap into pkg/errs), so we wrap the transient ones explicitly here.
func (c *Component) listSystemEntities(ctx context.Context, limit int) ([]string, error) {
	reqValue := sosa.SSNSystem
	reqBody, err := json.Marshal(struct {
		Predicate string  `json:"predicate"`
		Value     *string `json:"value,omitempty"`
		Limit     int     `json:"limit,omitempty"`
	}{
		Predicate: predicateRDFType,
		Value:     &reqValue,
		Limit:     limit,
	})
	if err != nil {
		return nil, errs.Wrap(err, "cs-api", "listSystemEntities", "marshal predicate query")
	}

	respBytes, err := c.nats.Request(ctx, subjectPredicateQuery, reqBody, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return nil, errs.WrapTransient(err, "cs-api", "listSystemEntities", "graph backend unavailable")
		case errors.Is(err, context.Canceled):
			// Caller went away. Surface as transient so /health does not
			// blame us, but the client will not see this response anyway.
			return nil, errs.WrapTransient(err, "cs-api", "listSystemEntities", "request cancelled")
		default:
			return nil, errs.Wrap(err, "cs-api", "listSystemEntities", "predicate query")
		}
	}

	var resp graph.QueryResponse[graph.PredicateData]
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, errs.Wrap(err, "cs-api", "listSystemEntities", "decode predicate response")
	}
	if resp.Error != "" {
		return nil, errs.WrapTransient(errors.New(resp.Error), "cs-api", "listSystemEntities", "predicate query")
	}
	return resp.Data.Entities, nil
}

// parseLimit validates ?limit= against the configured floor/ceiling. Empty
// input maps to defaultLimit; out-of-range values are an error.
func parseLimit(raw string, defaultLimit, maxLimit int) (int, error) {
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if n < 1 {
		return 0, fmt.Errorf("limit must be ≥ 1")
	}
	if n > maxLimit {
		return 0, fmt.Errorf("limit must be ≤ %d", maxLimit)
	}
	return n, nil
}

// writeBackendError maps an errs-classified error to an HTTP status. Mirrors
// the table in semstreams' gateway/http/README.md so behavior is uniform
// across gateways.
//
// Only 5xx responses bump c.errs — a stream of validation-shaped 400s from a
// confused client must not flip /health to 503 forever. /health treats
// c.errs as a *backend* health signal, not a client-traffic signal.
//
// 5xx error bodies do not echo err.Error() to the client to avoid leaking
// internal detail. The full error is logged with a generated request ID.
func (c *Component) writeBackendError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	body := "internal server error"
	switch {
	case errors.Is(err, errEntityNotFound):
		status = http.StatusNotFound
		body = err.Error() // safe to echo: the message is just "not found: <id>"
	case errs.IsInvalid(err):
		status = http.StatusBadRequest
		body = err.Error() // safe to echo: caller-supplied input was malformed
	case errs.IsTransient(err):
		status = http.StatusServiceUnavailable
		body = "service unavailable"
	}
	if status >= 500 {
		c.errs.Add(1)
	}
	c.logger.Warn("backend error", "err", err, "status", status)
	writeJSONError(w, status, body)
}
