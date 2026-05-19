package csapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// MediaType is a CS API response encoding the server can produce.
type MediaType string

const (
	MediaJSON    MediaType = "application/json"
	MediaJSONLD  MediaType = "application/ld+json"
	MediaGeoJSON MediaType = "application/geo+json"
	// SensorML 2.0 JSON encoding. Per CS API §11.7 the canonical media
	// type is `application/sml+json`; `application/sensorml+json` was a
	// pre-Stage-14 longer-name choice. We now serve the spec form;
	// MediaSensorMLLegacy is the long-name alias the supported() table
	// still advertises for backward compat with anything that learned
	// the long form from older /api docs or returned 406 bodies.
	MediaSensorML       MediaType = "application/sml+json"
	MediaSensorMLLegacy MediaType = "application/sensorml+json"
	MediaOMS            MediaType = "application/om+json"
	// OAS3 content types per OpenAPI Initiative registration (2021).
	// Used by /api (Stage 12) — Swagger UI / Redoc / openapi-generator
	// all prefer the +json form; the YAML form is the raw embedded body.
	MediaOAS3JSON MediaType = "application/vnd.oai.openapi+json;version=3.0"
	MediaOAS3YAML MediaType = "application/vnd.oai.openapi;version=3.0"
)

// ResourceFamily groups endpoints that share the same negotiable encoding set.
// Collection and item endpoints of the same kind do NOT share a family if
// their supported sets differ (e.g. there is no SensorML SystemCollection
// type, so the collection endpoint narrows). Adding a family is preferable
// to inline-narrowing in handlers because the 406 body advertises the
// correct supported set automatically.
type ResourceFamily int

const (
	FamilySystemItem            ResourceFamily = iota // GET /systems/{id}
	FamilySystemCollection                            // GET /systems
	FamilyDatastreamItem                              // GET /datastreams/{id}
	FamilyDatastreamCollection                        // GET /datastreams
	FamilyObservationCollection                       // GET /datastreams/{id}/observations (Stage 11)
	FamilySpatial
	FamilyService // /, /conformance
	FamilyAPI     // GET /api (Stage 12) — OAS3 service definition

	// FamilyProcedureCollection — GET /procedures. Stage 20 added the
	// resource; Stage 20.1 added MediaGeoJSON to the supported set
	// after the ETS's procedureFeatureHasGeoJsonSchemaAndMapping
	// assertion surfaced that `Accept: application/geo+json` is
	// expected to return a FeatureCollection (every Feature with
	// `geometry: null` per /req/procedure/location).
	FamilyProcedureCollection
	// FamilyProcedureItem — GET /procedures/{id}. JSON-only at v0.1.
	FamilyProcedureItem

	// FamilyDeploymentCollection — GET /deployments (Stage 21). JSON +
	// geo+json supported; deployments DO carry geometry (deploy site).
	FamilyDeploymentCollection
	// FamilyDeploymentItem — GET /deployments/{id}. JSON-only.
	FamilyDeploymentItem

	// FamilySamplingFeatureCollection — GET /samplingFeatures (Stage 22).
	// JSON + geo+json supported; sampling features carry first-class
	// geometry in the CS API Feature shape.
	FamilySamplingFeatureCollection
	// FamilySamplingFeatureItem — GET /samplingFeatures/{id}. JSON-only.
	FamilySamplingFeatureItem

	// FamilyPropertyCollection — GET /properties (Stage 23). JSON-only
	// at v0.1; the upstream schema is SensorML DerivedProperty-shaped
	// JSON, but this gateway returns the same JSON subset style used by
	// the other OSH-bar registry resources.
	FamilyPropertyCollection
	// FamilyPropertyItem — GET /properties/{id}. JSON-only.
	FamilyPropertyItem

	// FamilyControlStreamCollection — GET /controlstreams and
	// /systems/{id}/controlstreams (Stage 24). JSON-only at v0.1.
	FamilyControlStreamCollection
	// FamilyControlStreamItem — GET /controlstreams/{id}. JSON-only.
	FamilyControlStreamItem

	// FamilySystemEventCollection — GET /systemEvents and
	// /systems/{id}/events (Stage 25). JSON-only at v0.1.
	FamilySystemEventCollection
	// FamilySystemEventItem — GET /systemEvents/{id} and
	// /systems/{systemID}/events/{eventID}. JSON-only.
	FamilySystemEventItem
)

// supported returns the negotiable encodings for fam, in preference order.
// The first entry is the default when the client did not constrain Accept.
//
// Per-stage wiring:
//
//   - Stage 2:                FamilySystem = JSON
//   - Stage 3 (observations): + OMS on the consume side (POST), no
//     production-side encoder change (this table shapes responses)
//   - Stage 4 (this stage):   FamilySystem += SensorML + JSON-LD —
//     both encoders wired by GET /systems/{id}
//   - Stage 5 (spatial):      FamilySpatial += GeoJSON
//
// Until then, Negotiate honestly 406s on types we cannot serialise rather
// than promising bytes we can't produce.
func (fam ResourceFamily) supported() []MediaType {
	switch fam {
	case FamilySystemItem:
		// MediaSensorML (spec form `application/sml+json`) is ordered
		// before MediaSensorMLLegacy (`application/sensorml+json`) so
		// Negotiate's stable-sort tiebreaker picks the spec form when
		// a client sends both in the same Accept header (e.g.
		// `Accept: application/sml+json, application/sensorml+json`).
		// Long-form-only clients still get a 200 — they just match
		// the legacy entry second.
		return []MediaType{MediaJSON, MediaSensorML, MediaSensorMLLegacy, MediaJSONLD}
	case FamilySystemCollection:
		// No SensorML SystemCollection type; collection JSON-LD is a
		// follow-up (vocabulary/export is per-entity today). Stage 15
		// added MediaGeoJSON — `/systems` with Accept application/geo+json
		// returns an RFC 7946 FeatureCollection where each Feature
		// carries the system's cs-api.system.position (Stage 14) as
		// geometry. Required by the Botts ETS `geoJsonMediaTypeRead`
		// assertion.
		return []MediaType{MediaJSON, MediaGeoJSON}
	case FamilyDatastreamItem, FamilyDatastreamCollection:
		// Datastream is JSON-only at v0.1. The framework lacks
		// datastream vocabulary primitives (see
		// docs/upstream-asks/semstreams-datastream-vocabulary.md), so
		// JSON-LD would emit broken Linked Data; SWE Common 3.0 (the
		// natural datastream encoding) is in the framework's Scope-cut.
		// Distinct from FamilyObservationCollection so adding a future
		// datastream encoding (CoverageJSON, JSON-LD) doesn't drag the
		// observation negotiation set with it.
		return []MediaType{MediaJSON}
	case FamilyObservationCollection:
		// JSON returns the CS API §11.3 ObservationCollection wrapper
		// (numberMatched / items / links); OMS returns a bare array of
		// observation payloads for OMS-native clients. POST takes the
		// same OMS shape, so the round-trip is symmetric. Stage 11.
		return []MediaType{MediaJSON, MediaOMS}
	case FamilySpatial:
		// GeoJSON is the natural default for /areas — RFC 7946
		// FeatureCollection is the wire shape; clients asking for
		// plain JSON get the same bytes (FeatureCollection.MarshalJSON
		// is conformant either way) but with the application/json
		// Content-Type advertised.
		return []MediaType{MediaGeoJSON, MediaJSON}
	case FamilyService:
		return []MediaType{MediaJSON, MediaJSONLD}
	case FamilyProcedureCollection:
		// Stage 20.1 — geo+json wired to satisfy the ETS's
		// procedureFeatureHasGeoJsonSchemaAndMapping assertion. The
		// FeatureCollection emits each procedure as a Feature with
		// `geometry: null` per /req/procedure/location (procedures
		// have no location). Mirrors FamilySystemCollection's
		// geo+json wiring (Stage 15).
		return []MediaType{MediaJSON, MediaGeoJSON}
	case FamilyProcedureItem:
		// JSON-only at v0.1. SensorML on a procedure item would be
		// natural (Procedure ↔ SimpleProcess) but adds reverse-mapping
		// surface area not exercised by the ETS at v0.1; deferred.
		return []MediaType{MediaJSON}
	case FamilyDeploymentCollection:
		// Stage 21 — same JSON + geo+json shape /systems uses.
		// Deployments DO carry geometry (deploy site location).
		return []MediaType{MediaJSON, MediaGeoJSON}
	case FamilyDeploymentItem:
		return []MediaType{MediaJSON}
	case FamilySamplingFeatureCollection:
		// Stage 22 — mirrors deployments. SamplingFeature geometry is
		// first-class in the resource shape rather than an OSH-bar
		// workaround.
		return []MediaType{MediaJSON, MediaGeoJSON}
	case FamilySamplingFeatureItem:
		return []MediaType{MediaJSON}
	case FamilyPropertyCollection, FamilyPropertyItem:
		return []MediaType{MediaJSON}
	case FamilyControlStreamCollection, FamilyControlStreamItem:
		return []MediaType{MediaJSON}
	case FamilySystemEventCollection, FamilySystemEventItem:
		return []MediaType{MediaJSON}
	case FamilyAPI:
		// OAS3 JSON is the default — most OpenAPI tooling (Swagger UI,
		// Redoc, openapi-generator, kin-openapi) prefers it. MediaJSON
		// included as an alias so curl-without-Accept users get JSON
		// instead of 406. YAML is the alternate.
		return []MediaType{MediaOAS3JSON, MediaJSON, MediaOAS3YAML}
	default:
		return []MediaType{MediaJSON}
	}
}

// shortMediaNames maps the OGC API Common Part 1 §7 "?f=" short-name vocabulary
// to media types this gateway can produce. Common Part 1 defines `json` (and
// `html`, which we do not ship); the CS-API-specific encodings get the obvious
// 1:1 names so a client doesn't need to know Accept syntax to pick GeoJSON or
// SensorML. Keep this list aligned with the MediaXxx constants above.
var shortMediaNames = map[string]MediaType{
	"json":     MediaJSON,
	"geojson":  MediaGeoJSON,
	"sensorml": MediaSensorML,
	"om":       MediaOMS,
	"jsonld":   MediaJSONLD,
	// Stage 12: OAS3 short names for `/api?f=...`. `openapi` resolves to
	// the JSON form (matches the default); `yaml` resolves to the raw
	// embedded body. The OAS3 +json media type doesn't have an obvious
	// short name in the OpenAPI Initiative's docs, so we mint `openapi`
	// rather than coining something unusual.
	"openapi": MediaOAS3JSON,
	"yaml":    MediaOAS3YAML,
}

// NegotiateRequest is Negotiate's request-aware sibling. Per Common Part 1
// Conformance class "JSON" (req/json/content), the `?f=<short>` query parameter
// overrides Accept when present. Precedence is intentional: an explicit `?f=`
// short-name that does not map to fam's supported set 406s rather than falling
// through to Accept, because the override is a deliberate client signal.
//
// `?f=` absent, empty, or whitespace-only → fall back to Negotiate(Accept, fam).
// There is no `?f=` value that forces the family default independent of
// Accept — use `?f=<short>` explicitly if that's the intent.
//
// Unknown short names (not in shortMediaNames) 406, with the family's
// supported list advertised in the 406 body just like any other negotiation
// failure — clients sending `?f=html` against a JSON-only server need to see
// that html is not on offer.
func NegotiateRequest(r *http.Request, fam ResourceFamily) (MediaType, bool) {
	if short := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("f"))); short != "" {
		media, known := shortMediaNames[short]
		if !known {
			return "", false
		}
		for _, m := range fam.supported() {
			if m == media {
				return m, true
			}
		}
		return "", false
	}
	return Negotiate(r.Header.Get("Accept"), fam)
}

// Negotiate picks the response MediaType for an Accept header against fam's
// supported set. Returns (chosen, true) on success, ("", false) on 406 — the
// caller writes the 406 response (with the supported set in its body) so this
// function stays pure.
//
// HTTP handlers MUST use NegotiateRequest, not Negotiate — Negotiate alone
// skips the Common Part 1 `?f=` override and silently regresses the
// conformance class. Negotiate stays exported for the negotiation_test.go
// table (string-in / MediaType-out is trivial to drive in tables) and for
// any future non-HTTP negotiation caller.
func Negotiate(accept string, fam ResourceFamily) (MediaType, bool) {
	supported := fam.supported()
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return supported[0], true
	}

	type offer struct {
		media MediaType
		q     float64
		idx   int // index in supported — tiebreaker
	}
	var offers []offer

	parts := parseAccept(accept)
	for i, m := range supported {
		bestQ := -1.0
		for _, p := range parts {
			if !p.matches(string(m)) {
				continue
			}
			if p.q > bestQ {
				bestQ = p.q
			}
		}
		if bestQ > 0 {
			offers = append(offers, offer{m, bestQ, i})
		}
	}
	if len(offers) == 0 {
		return "", false
	}
	sort.SliceStable(offers, func(i, j int) bool {
		if offers[i].q != offers[j].q {
			return offers[i].q > offers[j].q
		}
		return offers[i].idx < offers[j].idx
	})
	return offers[0].media, true
}

// SupportedMedia returns the supported set for fam — for use in 406 bodies.
func SupportedMedia(fam ResourceFamily) []string {
	supported := fam.supported()
	out := make([]string, len(supported))
	for i, m := range supported {
		out[i] = string(m)
	}
	return out
}

// WriteNotAcceptable writes a 406 with the family's supported encoding list.
func WriteNotAcceptable(w http.ResponseWriter, fam ResourceFamily) {
	writeNotAcceptableList(w, SupportedMedia(fam))
}

// WriteNotAcceptableWith writes a 406 with an explicit supported list, used
// when the resource cannot satisfy its family's full claim (e.g. a System
// item missing rdf:type cannot be SensorML- or JSON-LD-rendered even though
// FamilySystemItem promises both).
func WriteNotAcceptableWith(w http.ResponseWriter, supported []MediaType) {
	out := make([]string, len(supported))
	for i, m := range supported {
		out[i] = string(m)
	}
	writeNotAcceptableList(w, out)
}

func writeNotAcceptableList(w http.ResponseWriter, supported []string) {
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusNotAcceptable)
	body := `{"error":"not acceptable","supported":["` +
		strings.Join(supported, `","`) + `"]}`
	_, _ = w.Write([]byte(body))
}

type acceptPart struct {
	typ, sub string
	q        float64
}

func (p acceptPart) matches(media string) bool {
	// Strip MIME parameters from the supported side — e.g. the OAS3
	// types `application/vnd.oai.openapi+json;version=3.0` carry an
	// informational `version=3.0` parameter that's part of the
	// canonical advertisement (Content-Type) but must not be load-
	// bearing for negotiation matching. Both sides drop parameters
	// before subtype comparison: the Accept side via parseAccept's
	// `;`-split that pulls only fields[0] into mediaSeg, and the
	// supported side here.
	if i := strings.IndexByte(media, ';'); i >= 0 {
		media = strings.TrimSpace(media[:i])
	}
	slash := strings.IndexByte(media, '/')
	if slash < 0 {
		return false
	}
	mt, ms := media[:slash], media[slash+1:]
	if p.typ != "*" && p.typ != mt {
		return false
	}
	if p.sub != "*" && p.sub != ms {
		return false
	}
	return true
}

func parseAccept(h string) []acceptPart {
	var out []acceptPart
	for _, raw := range strings.Split(h, ",") {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			continue
		}
		fields := strings.Split(seg, ";")
		mediaSeg := strings.TrimSpace(fields[0])
		slash := strings.IndexByte(mediaSeg, '/')
		if slash < 0 {
			continue
		}
		p := acceptPart{
			typ: strings.ToLower(mediaSeg[:slash]),
			sub: strings.ToLower(mediaSeg[slash+1:]),
			q:   1.0,
		}
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if !strings.HasPrefix(param, "q=") {
				continue
			}
			if q, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
				p.q = q
			}
		}
		out = append(out, p)
	}
	return out
}
