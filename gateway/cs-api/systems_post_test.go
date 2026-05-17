package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
)

// minimalSensorML produces a SensorML PhysicalSystem JSON body with
// uniqueId, label, and one identifier — the smallest input the gateway
// should accept and convert to a non-empty triple set.
func minimalSensorML(uniqueID, label string) []byte {
	body := map[string]any{
		"type":     "PhysicalSystem",
		"id":       "doc-id-1",
		"uniqueId": uniqueID,
		"label":    label,
		"identifiers": []map[string]any{
			{
				"definition": "http://example.org/serial",
				"label":      "Serial Number",
				"value":      "SN-0001",
			},
		},
	}
	out, _ := json.Marshal(body)
	return out
}

func encodeBatchOK(t *testing.T, written int) []byte {
	t.Helper()
	resp := graph.AddTriplesBatchResponse{
		MutationResponse: graph.MutationResponse{Success: true},
		WrittenCount:     written,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encodeBatchOK: %v", err)
	}
	return out
}

func encodeBatchPartialFailure(t *testing.T, failures map[string]string, written int) []byte {
	t.Helper()
	resp := graph.AddTriplesBatchResponse{
		MutationResponse: graph.MutationResponse{Success: false, Error: ""},
		WrittenCount:     written,
		FailedSubjects:   failures,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encodeBatchPartialFailure: %v", err)
	}
	return out
}

// TestHandleSystemPost_GoldenPath pins the happy-path contract: SensorML in,
// triples published to graph.mutation.triple.add_batch, 201 Created with
// Location header pointing at the minted entity ID.
func TestHandleSystemPost_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:11111111-2222-3333-4444-555555555555", "Test System")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != SubjectTripleAddBatch {
		t.Errorf("publish subject: got %q want %q", fake.gotSubject, SubjectTripleAddBatch)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/systems/"+c.cfg.SystemIDPrefix+".") {
		t.Errorf("Location: got %q, want /systems/<prefix>.<token>", loc)
	}
	// The minted token should derive from the uniqueId after URN scheme
	// stripping — UUID hyphens are preserved (they're in the allowed set).
	if !strings.HasSuffix(loc, "11111111-2222-3333-4444-555555555555") {
		t.Errorf("Location entity ID suffix: got %q, want UUID-derived suffix", loc)
	}

	// Body shape: { status: "created", id: ..., type: "System" }
	var body struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("body parse: %v", err)
	}
	if body.Status != "created" || body.Type != "System" {
		t.Errorf("body: got %+v", body)
	}
	if body.ID != strings.TrimPrefix(loc, "/systems/") {
		t.Errorf("body.ID %q != Location ID %q", body.ID, loc)
	}

	// Wire-shape check: the published body decodes as
	// graph.AddTriplesBatchRequest with all triples sharing the minted
	// entity ID as Subject.
	var sent graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &sent); err != nil {
		t.Fatalf("decode published body: %v", err)
	}
	if len(sent.Triples) == 0 {
		t.Fatal("no triples published")
	}
	for i, tr := range sent.Triples {
		if tr.Subject != body.ID {
			t.Errorf("triple[%d].Subject=%q want %q (all triples should share the entity ID)", i, tr.Subject, body.ID)
		}
	}
}

// TestHandleSystemPost_MissingUniqueIDMintsUUID proves the gateway mints a
// fresh UUID when SensorML uniqueId is absent (CS API §7.6: server may
// assign IDs). The Location header should still be 6-part-shaped.
func TestHandleSystemPost_MissingUniqueIDMintsUUID(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 2)}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("", "No-UniqueID System")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	// 6-part: prefix has 5 tokens + 1 minted = 6 dots-1 = 5 dots inside the ID
	idPart := strings.TrimPrefix(loc, "/systems/")
	if got, want := strings.Count(idPart, "."), 5; got != want {
		t.Errorf("minted ID dot count: got %d want %d (id=%q)", got, want, idPart)
	}
}

// TestHandleSystemPost_ContentTypeWrong returns 415 with Accept-Post
// header pointing at the supported media types.
//
// Stage 16 — accepted set expanded to include application/json +
// application/geo+json (GeoJSON Feature shape, CS API §7.6). Test
// uses application/xml as the wrong type to exercise the 415 branch.
func TestHandleSystemPost_ContentTypeWrong(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:x", "x")))
	req.Header.Set("Content-Type", "application/xml")
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415", rr.Code)
	}
	wantAcceptPost := strings.Join([]string{
		string(MediaSensorML), string(MediaSensorMLLegacy),
		string(MediaJSON), string(MediaGeoJSON),
	}, ", ")
	if got := rr.Header().Get("Accept-Post"); got != wantAcceptPost {
		t.Errorf("Accept-Post: got %q want %q", got, wantAcceptPost)
	}
	if fake.gotSubject != "" {
		t.Errorf("publish should not have been called; got subject=%q", fake.gotSubject)
	}
}

// TestHandleSystemPost_InvalidJSON returns 400 and does not publish.
func TestHandleSystemPost_InvalidJSON(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
	if fake.gotSubject != "" {
		t.Errorf("publish should not happen on decode failure; got=%q", fake.gotSubject)
	}
}

// TestHandleSystemPost_TransientBackend maps transport-layer NATS failures
// to 503 — the framework being unreachable is not the client's fault.
func TestHandleSystemPost_TransientBackend(t *testing.T) {
	fake := &fakeRequester{
		status:   natsclient.StatusConnected,
		replyErr: context.DeadlineExceeded,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:x", "x")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503", rr.Code)
	}
}

// TestHandleSystemPost_BatchPartialFailure maps a per-Subject failure from
// graph-ingest to 400 (the most common cause is entity-ID validation
// rejection, which is caller-correctable input).
func TestHandleSystemPost_BatchPartialFailure(t *testing.T) {
	failures := map[string]string{
		"c360.semconnect.systems.csapi.system.bad": "entity ID rejected: pattern mismatch",
	}
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchPartialFailure(t, failures, 0),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:bad", "Bad")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 (body=%s)", rr.Code, rr.Body.String())
	}
}

// TestHandleSystemPost_AuditHeadersPropagate proves that X-Forwarded-* on
// the inbound request lands on the NATS request's audit headers. Mirrors
// observations.go's audit pattern — even though graph-ingest doesn't
// capture these today, a trace-context audit subscriber needs them.
func TestHandleSystemPost_AuditHeadersPropagate(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 2)}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:audit", "Audit")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	req.Header.Set("X-Forwarded-User", "alice@example.org")
	req.Header.Set("X-Forwarded-Email", "alice@example.org")
	rr := httptest.NewRecorder()
	c.middleware(http.HandlerFunc(c.handleSystemPost)).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	if got := fake.gotHeaders["X-CS-Forwarded-User"]; got != "alice@example.org" {
		t.Errorf("X-CS-Forwarded-User: got %q want %q (headers=%v)", got, "alice@example.org", fake.gotHeaders)
	}
}

// TestUniqueIDToToken_EdgeCases pins the sanitizer behavior under inputs
// that the SensorML spec permits but our entity-ID grammar does not. The
// minted token MUST satisfy entityIDTokenRegex (validated end-to-end in
// the golden path, but pinned per-input here so a refactor that breaks
// the round-trip fails loudly on a single check.
func TestUniqueIDToToken_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // exact token, OR "" to skip exact-match (UUID minting case)
	}{
		{"empty mints UUID", "", ""},
		{"urn uuid stripped", "urn:uuid:abc-def-ghi", "abc-def-ghi"},
		// HTTP URI: scheme-strip removes only the leading "http:" (one
		// colon hop per iteration until none remain — port-style ":80"
		// in a different input would also be stripped). What's left,
		// "//example.org/sys/123", sanitizes to "_example_org_sys_123",
		// then leading-Trim drops the `_` pair → "example_org_sys_123".
		{"http URI", "http://example.org/sys/123", "example_org_sys_123"},
		{"dots replaced", "abc.def.ghi", "abc_def_ghi"},
		// Leading underscore is trimmed by strings.Trim — the surviving
		// "underscored" starts with a letter and matches the token regex.
		{"leading underscore trimmed", "_underscored", "underscored"},
		{"trim leading hyphens", "---abc---", "abc"},
		// All-symbols inputs Trim to "" → UUID branch.
		{"all-symbols input mints UUID", "!!!", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueIDToToken(tt.input)
			if !entityIDTokenRegex.MatchString(got) {
				t.Errorf("token %q does not satisfy entityIDTokenRegex", got)
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("token: got %q want %q", got, tt.want)
			}
		})
	}
}

// TestHandleSystemPost_HTTPCounted (regression for the middleware chain):
// the counting middleware must tick the request counter for POST 201
// responses. Pins symmetry with the GET counters so /health reports
// correct rates after POST traffic.
func TestHandleSystemPost_HTTPCounted(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 2)}
	c := newTestComponent(t, fake)

	before := c.requests.Load()
	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:x", "x")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.middleware(http.HandlerFunc(c.handleSystemPost)).ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201", rr.Code)
	}
	if c.requests.Load() != before+1 {
		t.Errorf("request counter: before=%d after=%d (expected +1)", before, c.requests.Load())
	}
}

// TestIngestTriples_PreCASValidationIs400 pins B-1 (review): when graph-ingest
// rejects the batch at the pre-CAS validation stage (Success=false,
// FailedSubjects empty), we map to 400. The framework's
// AddTriplesBatchResponse contract (graph/mutation_responses.go) is
// explicit that this is a caller-correctable input shape, NOT
// infrastructure unavailability — earlier wiring routed it to 503,
// which masked client bugs as ops noise.
func TestHandleSystemPost_PreCASValidation400(t *testing.T) {
	resp := graph.AddTriplesBatchResponse{
		MutationResponse: graph.MutationResponse{
			Success: false,
			Error:   "triple[0].Predicate empty",
		},
		WrittenCount:   0,
		FailedSubjects: nil,
	}
	respBytes, _ := json.Marshal(resp)
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: respBytes}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML("urn:uuid:precas", "PreCAS")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400 (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "triple[0].Predicate empty") {
		t.Errorf("body should forward framework error; got %s", rr.Body.String())
	}
}
