package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
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
	predicateRDFType      = "rdf.type"
)

// handleSystems serves GET /systems. CS API §7.13.
//
// Flow:
//  1. Negotiate Accept (Stage 2 wires JSON only — 406 for everything else;
//     SensorML / JSON-LD encoders land at Stage 4 and widen the supported set).
//  2. Parse ?limit= against the configured ceiling.
//  3. NATS request to graph.index.query.predicate filtering rdf:type = ssn:System.
//  4. Shape into CS API SystemCollection JSON.
func (c *Component) handleSystems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if _, ok := Negotiate(r.Header.Get("Accept"), FamilySystem); !ok {
		WriteNotAcceptable(w, FamilySystem)
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
