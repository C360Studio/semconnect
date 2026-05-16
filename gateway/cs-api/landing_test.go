package csapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
)

// TestHandleLanding_GoldenPath pins the Common-Core landing-page contract
// the Botts ETS exercises:
//   - 200 OK on GET / (req/core/root-success)
//   - body parses as JSON, Content-Type application/json (req/json/content)
//   - links[] contains self + conformance + at least one data link
//   - the conformance link advertises type="application/json"
//     (req/json/definition — commonLandingPageConformanceLinkHasJsonType)
func TestHandleLanding_GoldenPath(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	c.handleLanding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
	var body landingPage
	raw, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("response did not parse as JSON: %v; body=%s", err, raw)
	}
	if body.Title == "" {
		t.Error("title empty")
	}
	if body.Description == "" {
		t.Error("description empty")
	}

	rels := map[string][]link{}
	for _, l := range body.Links {
		rels[l.Rel] = append(rels[l.Rel], l)
	}

	if _, ok := rels["self"]; !ok {
		t.Errorf("missing self link; rels=%v", keys(rels))
	}
	// OGC API Common Part 1 §7.4 uses the SHORT relation name
	// `conformance`, not the OGC IRI. The first conformance harness run
	// against this PR failed `landingPageHasConformanceLink` because we
	// shipped the IRI; this pins the short form so a future regression
	// fails locally before the conformance gate.
	confLinks, ok := rels["conformance"]
	if !ok || len(confLinks) == 0 {
		t.Fatalf("missing conformance link (rel=conformance); rels=%v", keys(rels))
	}
	if confLinks[0].Type != string(MediaJSON) {
		t.Errorf("conformance link type: got %q want %q", confLinks[0].Type, MediaJSON)
	}

	dataLinks, ok := rels["data"]
	if !ok || len(dataLinks) == 0 {
		t.Fatalf("missing data link (rel=data); rels=%v", keys(rels))
	}
	// Hrefs are absolute (Stage 12). httptest's default request Host is
	// "example.com" without port, so the expected prefix is
	// http://example.com — we suffix-match to stay robust to that
	// (production absolute-URL composition tested in
	// TestAbsoluteBase_*).
	hrefSet := map[string]bool{}
	for _, l := range dataLinks {
		hrefSet[l.Href] = true
	}
	hasSuffix := func(suffix string) bool {
		for h := range hrefSet {
			if strings.HasSuffix(h, suffix) {
				return true
			}
		}
		return false
	}
	// Stage 12 added /datastreams to the data link set.
	for _, want := range []string{"/systems", "/datastreams", "/areas"} {
		if !hasSuffix(want) {
			t.Errorf("data link missing for %s; got %v", want, hrefSet)
		}
	}

	// Stage 12 — service-desc link is the unblock for the upstream-ETS
	// landingPageHasApiDefinitionLink + apiDefinitionResourceReturnsContent
	// assertions. Required when oas30 is declared in /conformance per
	// OGC API Common Part 1 §7.4.1 Table 4. Pin the exact shape so a
	// regression here fails locally before the conformance gate.
	svcDesc, ok := rels["service-desc"]
	if !ok || len(svcDesc) == 0 {
		t.Fatalf("missing service-desc link (Stage 12 oas30); rels=%v", keys(rels))
	}
	if !strings.HasSuffix(svcDesc[0].Href, "/api") {
		t.Errorf("service-desc href: got %q want suffix /api", svcDesc[0].Href)
	}
	if !strings.HasPrefix(svcDesc[0].Href, "http://") && !strings.HasPrefix(svcDesc[0].Href, "https://") {
		t.Errorf("service-desc href not absolute: got %q (Stage 12 portability requirement)", svcDesc[0].Href)
	}
	if svcDesc[0].Type != string(MediaOAS3JSON) {
		t.Errorf("service-desc type: got %q want %q", svcDesc[0].Type, MediaOAS3JSON)
	}
}

// TestHandleLanding_HEAD pins the HEAD semantics: status + Content-Type, no
// body. Matches the rest of the GET endpoints.
func TestHandleLanding_HEAD(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rr := httptest.NewRecorder()
	c.handleLanding(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if got := rr.Body.Len(); got != 0 {
		t.Errorf("HEAD body should be empty, got %d bytes: %s", got, rr.Body.String())
	}
}

// TestHandleLanding_FParameterHonored proves Common Part 1 §7
// (commonContentNegotiationHonoursFJsonParameter) — ?f=json wins over a
// non-JSON Accept, and ?f=unknown 406s instead of silently degrading.
func TestHandleLanding_FParameterHonored(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	// ?f=json wins over an Accept that would otherwise 406.
	req := httptest.NewRequest(http.MethodGet, "/?f=json", nil)
	req.Header.Set("Accept", "application/xml")
	rr := httptest.NewRecorder()
	c.handleLanding(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("?f=json status: got %d want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	// ?f=html (not implemented) 406s. The override is a deliberate
	// client signal — don't fall through to Accept.
	req = httptest.NewRequest(http.MethodGet, "/?f=html", nil)
	req.Header.Set("Accept", "application/json")
	rr = httptest.NewRecorder()
	c.handleLanding(rr, req)
	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("?f=html status: got %d want 406", rr.Code)
	}
}

// TestHandleLanding_RoutesRegistered proves the GET / route is mounted via
// the full mux (not just the handler directly), so the Go 1.22 `/{$}`
// pattern actually anchors at the bare root and doesn't shadow other
// routes or 404 the root.
func TestHandleLanding_RoutesRegistered(t *testing.T) {
	cases := []struct {
		name      string
		prefix    string
		root      string // path that should hit landing
		nonRoot   string // path that should still 404
		wantOther string // path that exists at this mount, expected 200/4xx but not 404
	}{
		{"standalone (empty prefix)", "", "/", "/sytems", "/conformance"},
		// ServiceManager-mounted case — landing lives at /cs-api/, NOT
		// bare /. Common Part 1 §6.1 frames "the landing page is the
		// entry point of the API", where "the API" is the cs-api
		// gateway, not the parent host.
		{"mounted at /cs-api", "/cs-api", "/cs-api/", "/cs-api/sytems", "/cs-api/conformance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			c := newTestComponent(t, fake)
			mux := http.NewServeMux()
			c.RegisterHTTPHandlers(tc.prefix, mux)

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.root, nil))
			if rr.Code != http.StatusOK {
				t.Errorf("GET %s: got %d want 200", tc.root, rr.Code)
			}

			// Unknown sibling MUST still 404 — the `{$}` end-of-path
			// anchor must not shadow other paths.
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.nonRoot, nil))
			if rr.Code != http.StatusNotFound {
				t.Errorf("GET %s: got %d want 404 (landing pattern is shadowing other paths)", tc.nonRoot, rr.Code)
			}

			// Known sibling at the same mount must NOT 404 — proves
			// the landing pattern coexists with other routes.
			rr = httptest.NewRecorder()
			mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.wantOther, nil))
			if rr.Code == http.StatusNotFound {
				t.Errorf("GET %s: got 404 (landing pattern may be over-matching)", tc.wantOther)
			}
		})
	}
}

func keys(m map[string][]link) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
