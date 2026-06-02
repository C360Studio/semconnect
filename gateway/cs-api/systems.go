package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/geo/geojson"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/vocabulary/export"
	"github.com/c360studio/semstreams/vocabulary/sosa"
	"github.com/nats-io/nats.go"
)

// CS API resource shape for the System collection. Stage 48 batch-hydrates
// name/description so Advanced Filtering clients can discover keyword
// evidence from the collection before narrowing with ?q.
type systemRef struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "System" per CS API §7.2 nominal class
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Links       []link `json:"links,omitempty"`
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
	NumberMatched  int  `json:"numberMatched"`
	NumberReturned int  `json:"numberReturned"`
	Truncated      bool `json:"truncated,omitempty"`
	// `items` (not `systems`) per CS API §7.13 / OGC API Common §7.14 items
	// resource. Stage 10 rename — the Botts ETS GeoJSON fixture loader
	// explicitly looks for the `items` array name and falls back to nothing
	// when missing ("/systems response has no CS API 'items' array").
	Items []systemRef `json:"items"`
	Links []link      `json:"links"`
}

// NATS subjects + the predicate-name constant the predicate index keys
// system / datastream entities under.
//
// CRITICAL: predicateClassType must match the predicate name actually
// written to triples — not the more obvious `"rdf.type"`. The framework's
// sensorml emitter (parser/sensorml/graphable.go) writes the type triple
// as `(entityID, "sensorml.process.type", <class IRI>)`, exposed as the
// `sensorml.PredType` constant. cs-api-server's POST /systems and POST
// /datastreams paths use that same constant when constructing their own
// type triples (gateway/cs-api/datastream.go datastreamToTriples). The
// predicate index keys those triples by the same name; querying with
// `"rdf.type"` returns zero entities — surfaced as Stage 12 conformance
// failure `systemsCollectionHasItemsArray` when the upstream-ETS core
// cascade unblocked. Stage 12 fixed.
const (
	subjectPredicateQuery = "graph.index.query.predicate"
	subjectEntityQuery    = "graph.query.entity"
	subjectBatchQuery     = "graph.query.batch"
	predicateClassType    = sensorml.PredType

	entityBatchChunkSize = 100
)

// system is the JSON shape returned by GET /systems/{id}. CS API §7.2's
// System resource has many more fields; v0.1 surfaces what the reverse
// mapping can populate without recursing into child entities. Lossy fields
// (inputs/outputs, keywords, connections, identifier metadata) are
// documented in gateway/cs-api/sensorml.go. Lossy-reconstruction signalling
// lives on the X-CS-Reconstructed-Lossy response header — single source so
// header and body cannot drift.
type system struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "System"
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Definition  string `json:"definition,omitempty"`
	// UID + UniqueID + FeatureProperties.UID preserve the
	// client-submitted identifier. The ETS's
	// {sensorMl,geoJson}MediaTypeWriteParsesSystemBodyWhenMutationEnabled
	// assertions check ANY of `uid` / `uniqueId` / `properties.uid` —
	// we surface the same value on all three from the same
	// framework uid triple so SensorML clients (`uniqueId`) and
	// Feature-shape clients (`properties.uid`) see the spelling they
	// expect. JSON-System readers see `uid` for backward symmetry
	// with the Feature POST body. All three are jointly present (uid
	// triple exists) or jointly absent (no triple) — no partial
	// states; tested via TestSystemFromState_NoUIDTriple_OmitsAllUIDFields.
	UID               string             `json:"uid,omitempty"`
	UniqueID          string             `json:"uniqueId,omitempty"`
	FeatureProperties *featureProperties `json:"properties,omitempty"`
	// Geometry is the GeoJSON-shaped position (`{type: Point, coordinates:
	// [lon, lat, alt?]}` typically) recovered from the framework position
	// triple. json.RawMessage so the round-trip preserves whatever shape
	// the client posted — Point, Polygon, etc.
	Geometry      json.RawMessage `json:"geometry,omitempty"`
	Hosts         []string        `json:"hosts,omitempty"`
	HostedBy      string          `json:"hostedBy,omitempty"`
	UsedProcedure string          `json:"usedProcedure,omitempty"`
	AttachedTo    string          `json:"attachedTo,omitempty"`
	Identifiers   []any           `json:"identifiers,omitempty"`
	Capabilities  []any           `json:"capabilities,omitempty"`
	// Characteristics carries SensorML `characteristics` lossy
	// reconstruction (Stage 4). Was named `properties` pre-Stage-18;
	// renamed to free the `properties` JSON key for the Feature-shape
	// container that the ETS expects. The data is identical, just
	// relocated. Documented via X-CS-Reconstructed-Lossy.
	Characteristics []any  `json:"characteristics,omitempty"`
	Links           []link `json:"links"`
}

// featureProperties mirrors the GeoJSON Feature-shape `properties`
// container — the same shape the client POSTs at /systems (Stage 16
// systemFeatureBody.Properties). Used on read so a Feature-aware
// client can pull `properties.{uid,name,description}` from a
// JSON-only response in the spelling the spec puts them.
//
// **Drift discipline**: all three fields are derived at marshal time
// from the same source triples (PredSystemUID → uid;
// sensorml.PredLabel → name; sensorml.PredDescription → description).
// Nothing stored separately. The "single-sourced" guarantee holds
// because systemFromState is the sole writer of this struct, and
// it reads name/description from the same top-level Label/Description
// fields the JSON System surfaces.
//
// Stage 19 NOTE: name was reinstated after the ETS's
// `systemsPatchLifecycleOptIn` test surfaced that
// `properties.name` is checked on the GET-after-PATCH; an earlier
// Stage 18 narrowing to uid-only broke that test. The "ETS only
// checks uid" narrowing from Stage 18 was correct for the
// {sensorMl,geoJson}MediaTypeWriteParsesSystemBodyWhenMutationEnabled
// assertions but missed the Update group's name check. Description
// re-added in symmetry; future fields land here when the ETS or a
// real client asks for them.
type featureProperties struct {
	UID                  string `json:"uid,omitempty"`
	Name                 string `json:"name,omitempty"`
	Description          string `json:"description,omitempty"`
	DeployedSystemsLinks []link `json:"deployedSystems@link,omitempty"`
	HostedProcedureLink  *link  `json:"hostedProcedure@link,omitempty"`
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
			// CS API §7 /req/system/canonical-url asserts a `rel=canonical`
			// link. We emit it pointing at the same URL as `self` —
			// canonical is the spec-mandated authoritative form (RFC 6596),
			// distinct from `self` (this representation). For us they
			// resolve to the same JSON URL since we don't host alternates
			// elsewhere. Stage 12 adds; surfaced by
			// `systemsCollectionLinksDiscipline` ETS assertion.
			{Href: "/systems/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			{Href: "/systems/" + state.ID, Rel: "alternate", Type: string(MediaSensorML)},
			{Href: "/systems/" + state.ID, Rel: "alternate", Type: string(MediaJSONLD)},
			{Href: "/systems/" + state.ID + "/datastreams", Rel: "datastreams", Type: string(MediaJSON)},
			{Href: "/systems/" + state.ID + "/controlstreams", Rel: "controlstreams", Type: string(MediaJSON)},
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
		s.Links = append(s.Links, link{
			Href:  "/systems/" + v,
			Rel:   "parent",
			Type:  string(MediaJSON),
			Title: "Parent system",
		})
	}
	// Surface position from the framework position triple. Object is
	// the raw GeoJSON-shaped JSON bytes (as string); cast back to
	// RawMessage so the JSON encoder writes them verbatim rather than
	// re-quoting them as a string literal.
	if v, ok := firstSystemPositionObject(state.Triples); ok {
		s.Geometry = json.RawMessage(v)
	}
	// Stage 18 — surface the preserved client-submitted identifier on
	// every spelling the ETS / spec clients look at. The same triple
	// feeds all three so they cannot drift.
	if v, ok := firstSystemUIDObject(state.Triples); ok {
		s.UID = v
		s.UniqueID = v
		s.FeatureProperties = &featureProperties{
			UID:         v,
			Name:        s.Label,
			Description: s.Description,
		}
	}
	s.Hosts = allStringObjects(state.Triples, sensorml.PredHosts)
	for _, t := range state.Triples {
		switch t.Predicate {
		case sensorml.PredIdentifierValue:
			s.Identifiers = append(s.Identifiers, t.Object)
		case sensorml.PredCapabilityValue:
			s.Capabilities = append(s.Capabilities, t.Object)
		case sensorml.PredCharacteristicValue:
			s.Characteristics = append(s.Characteristics, t.Object)
		}
	}
	return s
}

// handleSystems serves GET /systems. CS API §7.13.
//
// Flow:
//  1. Negotiate Accept across JSON (default — SystemCollection wrapper)
//     and GeoJSON (FeatureCollection wrapper, Stage 15).
//  2. Parse ?limit= against the configured ceiling.
//  3. NATS request to graph.index.query.predicate filtering rdf:type = ssn:System.
//  4. Shape into CS API SystemCollection JSON OR GeoJSON FeatureCollection.
//
// The GeoJSON path uses graph.query.batch to hydrate entity states and
// recover the framework position triple. Partial-success batch misses
// degrade to null-geometry Features rather than failing the whole request.
func (c *Component) handleSystems(w http.ResponseWriter, r *http.Request) {
	// Method is enforced by the ServeMux pattern ("GET /systems",
	// "HEAD /systems"); non-matching methods 405 before reaching here.
	media, ok := NegotiateRequest(r, FamilySystemCollection)
	if !ok {
		WriteNotAcceptable(w, FamilySystemCollection)
		return
	}

	query := r.URL.Query()
	limit, err := parseLimit(query.Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	filters, err := parseSystemCollectionFilters(query)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	candidateLimit := limit
	if filters.active() {
		candidateLimit = c.cfg.MaxListLimit
	}
	entities, err := c.listSystemEntities(r.Context(), candidateLimit)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if filters.active() {
		entities, err = c.filterSystemEntities(r.Context(), entities, filters, limit)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
	}

	if media == MediaGeoJSON {
		c.writeSystemsGeoJSON(w, r, entities, limit)
		return
	}

	coll := systemCollection{
		Type:           "SystemCollection",
		NumberMatched:  len(entities),
		NumberReturned: len(entities),
		Truncated:      len(entities) == limit, // see NumberMatched doc comment
		Items:          make([]systemRef, 0, len(entities)),
		Links: []link{
			{Href: "/systems", Rel: "self", Type: string(MediaJSON)},
		},
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), entities)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range entities {
		ref := systemRef{
			ID:   id,
			Type: "System",
			Links: []link{
				{Href: "/systems/" + id, Rel: "self", Type: string(MediaJSON)},
			},
		}
		if state, ok := statesByID[id]; ok {
			sys := systemFromState(state)
			ref.Name = sys.Label
			ref.Description = sys.Description
		} else {
			c.logger.Warn("batch entity fetch for SystemCollection missed entity; degrading to id-only item",
				"entity", id)
		}
		coll.Items = append(coll.Items, ref)
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

type wktPoint struct {
	lon float64
	lat float64
}

type systemCollectionFilters struct {
	idSet map[string]struct{}
	q     string
	geom  []wktPoint
}

func (f systemCollectionFilters) active() bool {
	return len(f.idSet) > 0 || f.q != "" || len(f.geom) > 0
}

func parseSystemCollectionFilters(values map[string][]string) (systemCollectionFilters, error) {
	var filters systemCollectionFilters
	ids, err := parseIDListFilter(values["id"])
	if err != nil {
		return filters, err
	}
	if len(ids) > 0 {
		filters.idSet = make(map[string]struct{}, len(ids))
		for _, id := range ids {
			filters.idSet[id] = struct{}{}
		}
	}
	if raw := strings.TrimSpace(firstQueryValue(values["q"])); raw != "" {
		filters.q = strings.ToLower(raw)
	}
	if raw := strings.TrimSpace(firstQueryValue(values["geom"])); raw != "" {
		geom, err := parseWKTPolygon(raw)
		if err != nil {
			return filters, fmt.Errorf("geom invalid: %w", err)
		}
		filters.geom = geom
	}
	return filters, nil
}

func firstQueryValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func parseIDListFilter(values []string) ([]string, error) {
	var ids []string
	var sawURI, sawLocal bool
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return nil, errors.New("id list must not contain empty values")
			}
			isURI := strings.Contains(part, "://")
			if isURI {
				sawURI = true
			} else {
				sawLocal = true
			}
			if sawURI && sawLocal {
				return nil, errors.New("id list must not mix local IDs and unique identifier URIs")
			}
			ids = append(ids, part)
		}
	}
	return ids, nil
}

func parseWKTPolygon(raw string) ([]wktPoint, error) {
	raw = strings.TrimSpace(raw)
	upper := strings.ToUpper(raw)
	if !strings.HasPrefix(upper, "POLYGON") {
		return nil, errors.New("only POLYGON WKT is supported")
	}
	body := strings.TrimSpace(raw[len("POLYGON"):])
	if !strings.HasPrefix(body, "((") || !strings.HasSuffix(body, "))") {
		return nil, errors.New("POLYGON must use double-parenthesized coordinate rings")
	}
	body = strings.TrimSpace(body[2 : len(body)-2])
	parts := strings.Split(body, ",")
	if len(parts) < 4 {
		return nil, errors.New("POLYGON outer ring must contain at least four positions")
	}
	points := make([]wktPoint, 0, len(parts))
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) < 2 {
			return nil, errors.New("POLYGON positions must contain longitude and latitude")
		}
		lon, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || !isFinite(lon) {
			return nil, fmt.Errorf("invalid longitude %q", fields[0])
		}
		lat, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || !isFinite(lat) {
			return nil, fmt.Errorf("invalid latitude %q", fields[1])
		}
		points = append(points, wktPoint{lon: lon, lat: lat})
	}
	first := points[0]
	last := points[len(points)-1]
	if first.lon != last.lon || first.lat != last.lat {
		return nil, errors.New("POLYGON outer ring must be closed")
	}
	return points, nil
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func (c *Component) filterSystemEntities(ctx context.Context, entities []string, filters systemCollectionFilters, limit int) ([]string, error) {
	statesByID, err := c.fetchEntitiesBatch(ctx, entities)
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(entities))
	for _, id := range entities {
		state, ok := statesByID[id]
		if !ok {
			continue
		}
		if systemStateMatchesFilters(state, filters) {
			filtered = append(filtered, id)
			if len(filtered) == limit {
				break
			}
		}
	}
	return filtered, nil
}

func systemStateMatchesFilters(state graph.EntityState, filters systemCollectionFilters) bool {
	sys := systemFromState(state)
	if len(filters.idSet) > 0 {
		if _, ok := filters.idSet[state.ID]; !ok {
			if sys.UID == "" {
				return false
			}
			if _, ok := filters.idSet[sys.UID]; !ok {
				return false
			}
		}
	}
	if filters.q != "" && !systemMatchesKeyword(sys, filters.q) {
		return false
	}
	if len(filters.geom) > 0 && !systemPointWithinWKTPolygon(sys, filters.geom) {
		return false
	}
	return true
}

func systemMatchesKeyword(sys system, q string) bool {
	for _, candidate := range []string{sys.ID, sys.UID, sys.UniqueID, sys.Label, sys.Description, sys.Definition} {
		if strings.Contains(strings.ToLower(candidate), q) {
			return true
		}
	}
	return false
}

func systemPointWithinWKTPolygon(sys system, polygon []wktPoint) bool {
	if len(sys.Geometry) == 0 {
		return false
	}
	geom, err := geojson.UnmarshalGeometry(sys.Geometry)
	if err != nil {
		return false
	}
	point, ok := geom.(geojson.Point)
	if !ok {
		return false
	}
	lon := point.Coordinates.Lon()
	lat := point.Coordinates.Lat()
	if !isFinite(lon) || !isFinite(lat) {
		return false
	}
	return pointInPolygon(wktPoint{lon: lon, lat: lat}, polygon)
}

func pointInPolygon(point wktPoint, polygon []wktPoint) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		pi := polygon[i]
		pj := polygon[j]
		if (pi.lat > point.lat) != (pj.lat > point.lat) {
			xAtY := (pj.lon-pi.lon)*(point.lat-pi.lat)/(pj.lat-pi.lat) + pi.lon
			if point.lon < xAtY {
				inside = !inside
			}
		}
	}
	return inside
}

// writeSystemsGeoJSON emits the GeoJSON FeatureCollection form of
// /systems. Stage 40 uses graph.query.batch for entity hydration.
// Missing entities from a partial-success batch reply log a warn and
// degrade to a Feature with null geometry; batch-level transport
// failures surface as backend errors.
func (c *Component) writeSystemsGeoJSON(w http.ResponseWriter, r *http.Request, entities []string, limit int) {
	features := make([]geojson.Feature, 0, len(entities))
	statesByID, err := c.fetchEntitiesBatch(r.Context(), entities)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range entities {
		idBytes, _ := json.Marshal(id)
		feature := geojson.Feature{
			RawID: idBytes,
			Properties: map[string]any{
				"id":   id,
				"type": "System",
			},
		}
		state, ok := statesByID[id]
		if !ok {
			c.logger.Warn("batch entity fetch for FeatureCollection missed entity; degrading to null geometry",
				"entity", id)
			features = append(features, feature)
			continue
		}
		sys := systemFromState(state)
		// Carry the system's reconstructed JSON fields as Feature
		// properties. Skip the `links` field — Features have their
		// own ID slot (RawID) and OGC GeoJSON consumers don't expect
		// nested CS-API link arrays inside Feature.properties.
		feature.Properties["label"] = sys.Label
		feature.Properties["description"] = sys.Description
		feature.Properties["definition"] = sys.Definition
		// Pluck the position triple (if present) as the Feature's
		// geometry.
		if posBytes, ok := firstSystemPositionObject(state.Triples); ok {
			if geom, gerr := geojson.UnmarshalGeometry([]byte(posBytes)); gerr == nil {
				feature.Geometry = geom
			} else {
				// Malformed position triple in storage — log and
				// emit Feature with null geometry. Don't poison the
				// whole page for one bad row.
				c.logger.Warn("malformed position triple; emitting null geometry",
					"entity", id, "err", gerr)
			}
		}
		features = append(features, feature)
	}
	fc := geojson.FeatureCollection{Features: features}

	w.Header().Set("Content-Type", string(MediaGeoJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if encErr := json.NewEncoder(w).Encode(fc); encErr != nil {
		c.errs.Add(1)
		c.logger.Error("encode systems FeatureCollection response", "err", encErr)
	}
}

// handleSystem serves GET /systems/{id}. CS API §7.2 (System resource).
//
// Flow:
//  1. Path-validate the entity ID (non-empty, NATS-token-safe — same rules
//     as datastream IDs since SemStreams 6-part IDs are NATS-shape).
//  2. Negotiate Accept across JSON / SensorML+JSON / JSON-LD.
//  3. NATS request to graph.query.entity to fetch the EntityState.
//  4. Classify framework handler failures via natsclient.ClassifyReply;
//     map entity-query "not found" to the local 404 sentinel.
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

	media, ok := NegotiateRequest(r, FamilySystemItem)
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
	case MediaSensorML, MediaSensorMLLegacy:
		// Both spec-form (sml+json) and long-form (sensorml+json) land
		// on the same reverse mapping; writeSystemSensorML emits the
		// negotiated media as its Content-Type so the response label
		// matches what the client asked for. Stage 14.
		c.writeSystemSensorML(w, r, state, media)
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
// or `sosa:Sensor`. An entity of any other rdf:type (Observation,
// FeatureOfInterest, Deployment, Procedure, …) is not a System and
// the URL space owes a 404.
//
// **Stage 20 NOTE:** `sosa.Procedure` was previously whitelisted
// here as a legacy concession for SensorML SimpleProcess /
// AggregateProcess bodies POSTed to /systems. With Stage 20
// minting genuine Procedure entities under /procedures, that
// concession is now a cross-resource collision: a procedure
// entity would be reachable through BOTH `/procedures/{id}` AND
// `/systems/{id}`. Removed. SimpleProcess bodies that need
// /systems-shaped serialization should POST to /procedures and
// be read back via /procedures/{id}.
func isSystemKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	switch typeIRI {
	case sosa.SSNSystem, sosa.Sensor:
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
func (c *Component) writeSystemSensorML(w http.ResponseWriter, r *http.Request, state graph.EntityState, media MediaType) {
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
	w.Header().Set("Content-Type", string(media))
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

	reply, err := c.nats.RequestWithHeaders(ctx, subjectEntityQuery, reqBody, nil, c.cfg.QueryTimeout)
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

	respBytes, err := natsclient.ClassifyReply(reply)
	if err != nil {
		classified := classifyEntityQueryFailure(err)
		return graph.EntityState{}, classified
	}

	var state graph.EntityState
	if err := json.Unmarshal(respBytes, &state); err != nil {
		return graph.EntityState{}, errs.Wrap(err, "cs-api", "fetchEntity", "decode entity state")
	}
	return state, nil
}

// fetchEntitiesBatch hydrates entity states via graph.query.batch in chunks.
// The backend returns partial successes by omitting missing IDs, so callers
// receive a map and decide whether a miss should skip or degrade a row.
func (c *Component) fetchEntitiesBatch(ctx context.Context, ids []string) (map[string]graph.EntityState, error) {
	statesByID := make(map[string]graph.EntityState, len(ids))
	for start := 0; start < len(ids); start += entityBatchChunkSize {
		end := start + entityBatchChunkSize
		if end > len(ids) {
			end = len(ids)
		}
		reqBody, err := json.Marshal(struct {
			IDs []string `json:"ids"`
		}{IDs: ids[start:end]})
		if err != nil {
			return nil, errs.Wrap(err, "cs-api", "fetchEntitiesBatch", "marshal batch query")
		}

		reply, err := c.nats.RequestWithHeaders(ctx, subjectBatchQuery, reqBody, nil, c.cfg.QueryTimeout)
		if err != nil {
			switch {
			case errors.Is(err, nats.ErrNoResponders),
				errors.Is(err, nats.ErrTimeout),
				errors.Is(err, context.DeadlineExceeded),
				errors.Is(err, nats.ErrConnectionClosed):
				return nil, errs.WrapTransient(err, "cs-api", "fetchEntitiesBatch", "graph backend unavailable")
			default:
				return nil, errs.Wrap(err, "cs-api", "fetchEntitiesBatch", "batch query")
			}
		}

		respBytes, err := natsclient.ClassifyReply(reply)
		if err != nil {
			if errs.IsInvalid(err) {
				return nil, errs.WrapInvalid(err, "cs-api", "fetchEntitiesBatch", "bad query")
			}
			if errs.IsTransient(err) {
				return nil, errs.WrapTransient(err, "cs-api", "fetchEntitiesBatch", "graph backend unavailable")
			}
			return nil, errs.Wrap(err, "cs-api", "fetchEntitiesBatch", "backend error")
		}

		var resp struct {
			Entities []graph.EntityState `json:"entities"`
		}
		if err := json.Unmarshal(respBytes, &resp); err != nil {
			return nil, errs.Wrap(err, "cs-api", "fetchEntitiesBatch", "decode batch entity states")
		}
		for _, state := range resp.Entities {
			if state.ID == "" {
				continue
			}
			statesByID[state.ID] = state
		}
	}
	return statesByID, nil
}

// errEntityNotFound is the sentinel writeBackendError translates to 404.
// pkg/errs has no NotFound class today (Invalid / Transient / Fatal only);
// rather than overload Invalid → 400 to also mean "missing entity → 404",
// we keep a local sentinel here and have writeBackendError detect it.
var errEntityNotFound = errors.New("cs-api: entity not found")
var errEntityConflict = errors.New("cs-api: entity already exists")

// classifyEntityQueryFailure maps semstreams' header-classified
// graph.query.entity handler errors into CS API HTTP semantics. beta.87
// carries X-Error-Class via natsclient.ClassifyReply, but "not found" is
// intentionally still a gateway-level distinction within the Invalid class.
//
// During the additive #93 window, ClassifyReply also handles legacy
// "error: ..." bodies from older handlers, so the mapping remains backward
// compatible with pre-beta.87 replies.
func classifyEntityQueryFailure(err error) error {
	if err == nil {
		return nil
	}
	tail := strings.TrimPrefix(err.Error(), "error: ")
	switch {
	case strings.HasPrefix(tail, "not found:"):
		return fmt.Errorf("%w: %s", errEntityNotFound, tail)
	case errs.IsInvalid(err):
		return errs.WrapInvalid(err, "cs-api", "fetchEntity", "bad query")
	case errs.IsTransient(err):
		return errs.WrapTransient(err, "cs-api", "fetchEntity", "graph backend unavailable")
	default:
		return errs.Wrap(err, "cs-api", "fetchEntity", "backend error")
	}
}

// listSystemEntities issues a predicate query for every ssn:System entity
// and returns the matching IDs. Thin wrapper around listEntitiesByType
// (Stage 8 generalized the predicate-query path to also serve /datastreams).
func (c *Component) listSystemEntities(ctx context.Context, limit int) ([]string, error) {
	return c.listEntitiesByType(ctx, sosa.SSNSystem, limit, "listSystemEntities")
}

// listEntitiesByType is the shared predicate-query helper: rdf:type = typeIRI,
// limit, returns entity IDs. Used by GET /systems and GET /datastreams.
//
// Errors are classified at this boundary so writeBackendError downstream can
// map cleanly to HTTP status. natsclient returns raw nats sentinels (it does
// NOT wrap into pkg/errs), so we wrap the transient ones explicitly here.
// opName names the calling endpoint so log lines stay distinguishable.
func (c *Component) listEntitiesByType(ctx context.Context, typeIRI string, limit int, opName string) ([]string, error) {
	reqValue := typeIRI
	reqBody, err := json.Marshal(struct {
		Predicate string  `json:"predicate"`
		Value     *string `json:"value,omitempty"`
		Limit     int     `json:"limit,omitempty"`
	}{
		Predicate: predicateClassType,
		Value:     &reqValue,
		Limit:     limit,
	})
	if err != nil {
		return nil, errs.Wrap(err, "cs-api", opName, "marshal predicate query")
	}

	respBytes, err := c.nats.Request(ctx, subjectPredicateQuery, reqBody, c.cfg.QueryTimeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders),
			errors.Is(err, nats.ErrTimeout),
			errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrConnectionClosed):
			return nil, errs.WrapTransient(err, "cs-api", opName, "graph backend unavailable")
		case errors.Is(err, context.Canceled):
			// Caller went away. Surface as transient so /health does not
			// blame us, but the client will not see this response anyway.
			return nil, errs.WrapTransient(err, "cs-api", opName, "request cancelled")
		default:
			return nil, errs.Wrap(err, "cs-api", opName, "predicate query")
		}
	}

	var resp graph.QueryResponse[graph.PredicateData]
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, errs.Wrap(err, "cs-api", opName, "decode predicate response")
	}
	if resp.Error != "" {
		return nil, errs.WrapTransient(errors.New(resp.Error), "cs-api", opName, "predicate query")
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
	case errors.Is(err, errEntityConflict):
		status = http.StatusConflict
		body = err.Error() // safe to echo: duplicate entity ID
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
