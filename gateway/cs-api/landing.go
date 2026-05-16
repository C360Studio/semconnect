package csapi

import (
	"encoding/json"
	"net/http"
)

// landingPage is the JSON document served at GET /. OGC API Common Part 1
// §7.2 (req/core/root-success) requires a landing page at the API root.
// `service-desc` / `service-doc` are deliberately absent — see the OAS30
// deferral note in conformance.go.
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
	// Hrefs are relative — dereferenceable against the request URL per
	// RFC 3986. Production deployments behind a TLS-terminating proxy
	// would want absolute URLs built from X-Forwarded-Proto/Host; tracked
	// as a v0.2 follow-up rather than guessed at here.
	body := landingPage{
		Title:       "Connected Systems gateway (semconnect)",
		Description: "OGC API Connected Systems v1.0 gateway over the semstreams framework.",
		Links: []link{
			{Href: "/", Rel: "self", Type: string(MediaJSON), Title: "this document"},
			{Href: "/conformance", Rel: "conformance", Type: string(MediaJSON), Title: "conformance declaration"},
			{Href: "/systems", Rel: "data", Type: string(MediaJSON), Title: "system collection"},
			{Href: "/areas", Rel: "data", Type: string(MediaGeoJSON), Title: "spatial filter (areas)"},
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
