package csapi

import (
	"encoding/json"
	"net/http"
)

// absoluteBase returns a URL prefix like `https://api.example.com` (no
// trailing slash) suitable for prepending to root-relative paths when
// building absolute hrefs in landing-page / link responses. Stage 12.
//
// Precedence:
//
//  1. X-Forwarded-Proto + X-Forwarded-Host — set by a trusting reverse
//     proxy (nginx, traefik, ingress-nginx, etc.). Operators relying
//     on TLS termination at the proxy MUST forward these for clients
//     to receive a usable absolute URL.
//  2. r.TLS != nil → https, else http; host from r.Host (the value
//     the client put on the request line, normalized to include port
//     when non-default).
//
// We do not consult Forwarded (RFC 7239) at v0.1 — its parsing
// surface is large and the X-Forwarded-* duo covers every reverse
// proxy we deploy behind today. Adding Forwarded is a one-block
// extension here when needed.
func absoluteBase(r *http.Request) string {
	scheme := "http"
	// Whitelist X-Forwarded-Proto to {http, https} — a buggy proxy or
	// hostile testing harness sending Proto=javascript:alert(1) would
	// otherwise produce a junk scheme in every link. Real proxies only
	// ever set http or https here; anything else is treated as missing
	// and we fall through to direct-detection.
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

// landingPage is the JSON document served at GET /. OGC API Common Part 1
// §7.2 (req/core/root-success) requires a landing page at the API root.
// Stage 12 added the `service-desc` link pointing at /api now that we ship
// an OAS3 service definition and claim the `oas30` conformance class.
type landingPage struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Links       []link `json:"links"`
}

// handleLanding serves GET /{$} (the bare root path, exact match — Go 1.22+
// `/{$}` pattern excludes the prefix-style "/" trap that would shadow
// every other route).
func (c *Component) handleLanding(w http.ResponseWriter, r *http.Request) {
	// Always JSON for v0.1. FamilyService claims JSON+JSON-LD; the
	// landing page is JSON-shaped (not RDF), so we honor only JSON
	// here. NegotiateRequest still handles ?f=json per Common Part 1
	// §7 (req/json/content) — see negotiation.go.
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}

	// OGC API Common Part 1 §7.4 specifies the SHORT relation names
	// `conformance` and `data` (not the full http://www.opengis.net/def/rel/...
	// IRIs). The Botts ETS asserts `rel=conformance` literally, and v0.1
	// landing-page consumers (including pygeoapi / ldproxy clients) all
	// expect the short form. Don't substitute the URI here.
	//
	// Stage 12 made hrefs ABSOLUTE (was: root-relative). Some clients —
	// notably the Botts ETS via REST Assured `given().when().get(URI)`
	// — don't auto-resolve a relative URI against the document's own
	// URL, instead bare-fetching it which hits the wrong server (or
	// 404s). RFC 3986 §5.2 allows resolution, but in-practice
	// portability means absolute. We build the base from
	// X-Forwarded-{Proto,Host} when present (reverse-proxy case) and
	// fall back to the request's own scheme + Host.
	base := absoluteBase(r)
	body := landingPage{
		Title:       "Connected Systems gateway (semconnect)",
		Description: "OGC API Connected Systems v1.0 gateway over the semstreams framework.",
		Links: []link{
			{Href: base + "/", Rel: "self", Type: string(MediaJSON), Title: "this document"},
			{Href: base + "/conformance", Rel: "conformance", Type: string(MediaJSON), Title: "conformance declaration"},
			// Stage 12 — required when oas30 is claimed in /conformance,
			// per OGC API Common Part 1 §7.4.1 Table 4. cs-api ships the
			// OAS3 document at /api (see gateway/cs-api/api.go +
			// gateway/cs-api/openapi.yaml).
			{Href: base + "/api", Rel: "service-desc", Type: string(MediaOAS3JSON), Title: "OpenAPI 3.0 service definition"},
			{Href: base + "/systems", Rel: "data", Type: string(MediaJSON), Title: "system collection"},
			{Href: base + "/datastreams", Rel: "data", Type: string(MediaJSON), Title: "datastream collection"},
			{Href: base + "/areas", Rel: "data", Type: string(MediaGeoJSON), Title: "spatial filter (areas)"},
			// Stage 12 — CS API §7.6 / OGC Common Part 2 "Collections":
			// resource-specific rels signal that a top-level collection
			// endpoint is the landing-page-resolvable entry point for that
			// resource family. The Botts ETS asserts
			// `/req/system/collections` via either `/collections` (Part 2
			// implementation, deferred at v0.1) OR a `rel=systems`/
			// `rel=collection`/`rel=collections` link here. Same for
			// datastreams. These are additive — `rel=data` stays for
			// OGC Common Part 1 compatibility.
			{Href: base + "/systems", Rel: "systems", Type: string(MediaJSON), Title: "system resources"},
			{Href: base + "/datastreams", Rel: "datastreams", Type: string(MediaJSON), Title: "datastream resources"},
			// Stage 25 — Part 2 API Common discovery. The pinned ETS
			// uses landing-page links whose rel/path tokens name Part 2
			// resource collections, then verifies collection JSON
			// exposes items[] and links[].
			{Href: base + "/controlstreams", Rel: "controlstreams", Type: string(MediaJSON), Title: "control stream resources"},
			{Href: base + "/commands", Rel: "commands", Type: string(MediaJSON), Title: "command resources"},
			{Href: base + "/systemEvents", Rel: "systemevents", Type: string(MediaJSON), Title: "system event resources"},
		},
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("encode landing response", "err", err)
	}
}
