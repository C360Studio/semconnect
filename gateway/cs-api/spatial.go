package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/c360studio/semstreams/graph/geo/geojson"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go"
)

// NATS subjects for the framework's spatial-index queries (ADR-044 Phase 3).
const (
	subjectSpatialBounds  = "graph.spatial.query.bounds"
	subjectSpatialPolygon = "graph.spatial.query.polygon"
)

// spatialResult mirrors graph-index-spatial's SpatialResult wire shape. The
// framework only returns ID + Type ("entity"); coordinates remain in the
// spatial index and are NOT echoed in the response. v0.1 emits Features with
// geometry=null (RFC 7946 §3.2 explicitly permits this); recovering precise
// coordinates per matching entity requires a follow-up upstream change to
// SpatialResult (file an issue if a downstream consumer needs them).
type spatialResult struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// boundsQuery is the request envelope the framework's bounds handler expects.
// Fields mirror processor/graph-index-spatial/query.go boundsRequest.
type boundsQuery struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
	Limit int     `json:"limit"`
}

// polygonQuery is the request envelope the framework's polygon handler expects.
// Polygon is RawMessage so the framework can dispatch through
// geojson.UnmarshalGeometry with type-discriminator validation.
type polygonQuery struct {
	Polygon json.RawMessage `json:"polygon"`
	Limit   int             `json:"limit"`
}

// handleAreas serves GET /areas — spatial filtering across all indexed
// entities. CS API §6.5 (and OGC API Features §7.13). The query is shaped by
// either ?bbox or ?polygon (exactly one). Optional ?limit caps the result.
//
// Response: GeoJSON FeatureCollection. Each Feature has the entity's ID and
// geometry=null (RFC 7946 §3.2). Clients drill via GET /systems/{id} for the
// entity's precise location until upstream extends SpatialResult.
//
// Errors:
//   - 400 if neither bbox nor polygon, or both, or malformed parse
//   - 503 if the spatial backend is unavailable
//   - 406 if Accept names a type FamilySpatial does not support
func (c *Component) handleAreas(w http.ResponseWriter, r *http.Request) {
	// Method enforced by the ServeMux pattern.
	media, ok := NegotiateRequest(r, FamilySpatial)
	if !ok {
		WriteNotAcceptable(w, FamilySpatial)
		return
	}

	bboxRaw := r.URL.Query().Get("bbox")
	polyRaw := r.URL.Query().Get("polygon")
	if bboxRaw == "" && polyRaw == "" {
		writeJSONError(w, http.StatusBadRequest, "exactly one of ?bbox or ?polygon required")
		return
	}
	if bboxRaw != "" && polyRaw != "" {
		writeJSONError(w, http.StatusBadRequest, "?bbox and ?polygon are mutually exclusive")
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var results []spatialResult
	switch {
	case bboxRaw != "":
		bbox, err := parseBBox(bboxRaw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		results, err = c.querySpatialBounds(r.Context(), bbox, limit)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
	case polyRaw != "":
		poly, err := parsePolygon(polyRaw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		results, err = c.querySpatialPolygon(r.Context(), poly, limit)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
	}

	fc := buildFeatureCollection(results)

	switch media {
	case MediaJSON:
		// Same FeatureCollection bytes, advertised as plain JSON for
		// clients that asked for it. FeatureCollection.MarshalJSON
		// produces RFC-7946-conformant JSON either way.
		w.Header().Set("Content-Type", string(MediaJSON))
	default:
		w.Header().Set("Content-Type", string(MediaGeoJSON))
	}
	w.Header().Set("X-CS-Geometry-Available", "false") // see file doc — Stage 5 limitation
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(fc); err != nil {
		c.errs.Add(1)
		c.logger.Error("encode FeatureCollection", "err", err)
	}
}

// buildFeatureCollection emits one Feature per spatial result with
// geometry=null. The Feature.id carries the entity ID; Properties carries
// the spatial-result type tag ("entity") and a self link clients can follow.
func buildFeatureCollection(results []spatialResult) geojson.FeatureCollection {
	features := make([]geojson.Feature, 0, len(results))
	for _, r := range results {
		idBytes, _ := json.Marshal(r.ID)
		features = append(features, geojson.Feature{
			Geometry: nil, // RFC 7946 §3.2: null geometry permitted
			RawID:    idBytes,
			Properties: map[string]any{
				"type": r.Type,
				"href": "/systems/" + r.ID,
			},
		})
	}
	return geojson.FeatureCollection{Features: features}
}

// parseBBox parses CSV "minLon,minLat,maxLon,maxLat" per RFC 7946 §5 / OGC
// API Features §7.15.3. Optional 6-value form (with elevation) is rejected
// at v0.1 — the framework's spatial index doesn't carry vertical extent.
func parseBBox(raw string) (boundsQuery, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return boundsQuery{}, fmt.Errorf("bbox must be 4 comma-separated values (minLon,minLat,maxLon,maxLat), got %d", len(parts))
	}
	vals := make([]float64, 4)
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return boundsQuery{}, fmt.Errorf("bbox value %d is not a number: %q", i, p)
		}
		// NaN/Inf bypass the range guards below: IEEE 754 says
		// `NaN < x`, `NaN > x`, and `NaN > NaN` are all false, so a
		// NaN-laced bbox would sneak past every comparison and reach
		// the framework's geohash math with undefined results. Reject
		// non-finite values explicitly.
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return boundsQuery{}, fmt.Errorf("bbox value %d is not a finite number: %q", i, p)
		}
		vals[i] = v
	}
	minLon, minLat, maxLon, maxLat := vals[0], vals[1], vals[2], vals[3]
	if minLon < -180 || minLon > 180 || maxLon < -180 || maxLon > 180 {
		return boundsQuery{}, errors.New("bbox longitudes must be within [-180, 180]")
	}
	if minLat < -90 || minLat > 90 || maxLat < -90 || maxLat > 90 {
		return boundsQuery{}, errors.New("bbox latitudes must be within [-90, 90]")
	}
	if minLat > maxLat {
		return boundsQuery{}, errors.New("bbox minLat exceeds maxLat")
	}
	if minLon > maxLon {
		// Antimeridian-crossing bboxes are RFC-legal but require
		// client-side splitting; the framework's spatial index does
		// not auto-split. Reject explicitly with a pointer to the
		// workaround so a client sees a useful 400, not a no-results
		// quiet success.
		return boundsQuery{}, errors.New("bbox crossing antimeridian not supported — split client-side at ±180°")
	}
	return boundsQuery{
		West:  minLon,
		South: minLat,
		East:  maxLon,
		North: maxLat,
	}, nil
}

// parsePolygon URL-decodes and JSON-decodes a GeoJSON Polygon. The framework's
// geojson.UnmarshalGeometry does NOT validate ring shape — see
// graph/geo/geojson/json.go: it just json.Unmarshals into Polygon{Coordinates:
// [][]Position}. A polygon with zero rings, < 4 points per ring, or a
// non-closed ring will pass UnmarshalGeometry and reach Polygon.Contains, which
// degrades silently on malformed input. We enforce RFC 7946 §3.1.6 here so the
// only spatial-query failures left for the framework are "internal error"
// shapes — which deserve their 500.
func parsePolygon(raw string) (json.RawMessage, error) {
	// http.URL.Query() already URL-decoded the value, so raw is the
	// decoded GeoJSON JSON. Validate just enough to fail fast on bad
	// input before paying the NATS round-trip.
	if !json.Valid([]byte(raw)) {
		return nil, errors.New("polygon parameter is not valid JSON")
	}
	geom, err := geojson.UnmarshalGeometry([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("polygon parameter is not a valid GeoJSON geometry: %w", err)
	}
	poly, ok := geom.(geojson.Polygon)
	if !ok {
		return nil, fmt.Errorf("polygon parameter is %q, expected Polygon", geom.Type())
	}
	if err := validatePolygonRings(poly); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// validatePolygonRings enforces RFC 7946 §3.1.6: a Polygon has ≥ 1 linear
// ring; each ring has ≥ 4 positions; each ring's first and last positions
// are equal (closure). The framework does none of these checks.
func validatePolygonRings(p geojson.Polygon) error {
	if len(p.Coordinates) == 0 {
		return errors.New("polygon must have at least one (outer) ring")
	}
	for i, ring := range p.Coordinates {
		if len(ring) < 4 {
			return fmt.Errorf("polygon ring %d has %d points; RFC 7946 §3.1.6 requires ≥ 4", i, len(ring))
		}
		first, last := ring[0], ring[len(ring)-1]
		if first.Lon() != last.Lon() || first.Lat() != last.Lat() {
			return fmt.Errorf("polygon ring %d is not closed (first ≠ last)", i)
		}
	}
	return nil
}

// querySpatialBounds issues the NATS request and decodes the bare
// []SpatialResult response. The bounds handler does NOT wrap its response in
// a QueryResponse envelope (unlike graph.index.query.predicate); we decode
// the array directly. Error classification follows the Stage-2/3/4 pattern.
func (c *Component) querySpatialBounds(ctx context.Context, q boundsQuery, limit int) ([]spatialResult, error) {
	q.Limit = limit
	body, err := json.Marshal(q)
	if err != nil {
		return nil, errs.Wrap(err, "cs-api", "querySpatialBounds", "marshal bounds query")
	}
	return c.runSpatialQuery(ctx, subjectSpatialBounds, body)
}

// querySpatialPolygon issues the NATS request for polygon-containment.
func (c *Component) querySpatialPolygon(ctx context.Context, poly json.RawMessage, limit int) ([]spatialResult, error) {
	body, err := json.Marshal(polygonQuery{Polygon: poly, Limit: limit})
	if err != nil {
		return nil, errs.Wrap(err, "cs-api", "querySpatialPolygon", "marshal polygon query")
	}
	return c.runSpatialQuery(ctx, subjectSpatialPolygon, body)
}

// runSpatialQuery handles the shared NATS request/decode for both spatial
// subjects. The framework returns a bare JSON array on success.
//
// Note on the framework-error workaround: graph-index-spatial wraps its
// handler errors via errs.WrapInvalid, which produces wire replies shaped
// "error: Component.<method>: <action> failed: …" — NOT the
// "error: <kind>: …" prefix that graph-ingest emits. classifyEntityQueryError
// (used on the /systems/{id} path) cannot disambiguate these because the
// prefix structure is different. Instead we rely on tight client-side
// validation in parseBBox / parsePolygon so the framework never receives
// invalid input from us; any "error:" reply from this subject therefore
// represents a server-side fault and falls through to the generic decode
// failure → 500 below. Tracked in the upstream issue covering both the
// error-format divergence and the SpatialResult coordinates gap.
func (c *Component) runSpatialQuery(ctx context.Context, subject string, body []byte) ([]spatialResult, error) {
	respBytes, err := c.nats.Request(ctx, subject, body, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return nil, errs.WrapTransient(err, "cs-api", "runSpatialQuery", "spatial backend unavailable")
		default:
			return nil, errs.Wrap(err, "cs-api", "runSpatialQuery", "spatial query")
		}
	}
	var results []spatialResult
	if err := json.Unmarshal(respBytes, &results); err != nil {
		return nil, errs.Wrap(err, "cs-api", "runSpatialQuery", "decode spatial response")
	}
	return results, nil
}
