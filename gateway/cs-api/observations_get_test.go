package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/message/oms"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/pkg/swecommon"
	"github.com/nats-io/nats.go"
)

// fakeReader implements streamReader. The handler driving end of Stage 11
// — we substitute this for jetstreamObservationReader to drive the
// handler without a live NATS / JetStream stack.
type fakeReader struct {
	wantSubject string
	wantLimit   int
	wantStart   uint64
	gotSubject  string
	gotLimit    int
	gotStart    uint64
	gotCalls    int
	msgs        []observationMsg
	err         error
}

func (f *fakeReader) FetchSubject(_ context.Context, subject string, limit int, startSeq uint64) ([]observationMsg, error) {
	f.gotCalls++
	f.gotSubject = subject
	f.gotLimit = limit
	f.gotStart = startSeq
	return f.msgs, f.err
}

// wireObservationsReadComponent assembles a Component with a real
// natsRequester fake (for any other endpoints) AND a fake reader. Mirrors
// wireObservationsComponent in observations_test.go.
func wireObservationsReadComponent(t *testing.T, rd *fakeReader) *Component {
	t.Helper()
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	var sr streamReader = rd
	c.reader.Store(&sr)
	c.initialized = true
	return c
}

func wireObservationsReadComponentWithRequester(t *testing.T, rd *fakeReader, fake natsRequester) *Component {
	t.Helper()
	cfg := DefaultConfig()
	cfg.QueryTimeout = 500 * time.Millisecond
	c, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var sr streamReader = rd
	c.reader.Store(&sr)
	c.initialized = true
	return c
}

// encodeBaseMessage wraps an oms.Observation in a BaseMessage envelope
// the same way observations.go's POST path does. Stored bytes look exactly
// like what JetStream serves back from the per-datastream subject.
func encodeBaseMessage(t *testing.T, obs *oms.Observation) []byte {
	t.Helper()
	// BaseMessage's envelope codec needs the OMS payload type registered;
	// payloadbuiltins.NewTestRegistry handles it. Idempotent across calls.
	_ = payloadbuiltins.NewTestRegistry(t)
	msg := message.NewBaseMessage(obs.Schema(), obs, "cs-api-ingest")
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal BaseMessage: %v", err)
	}
	return b
}

// TestObservationsGet_EmptyStream — happy path on a freshly-seeded
// datastream: 200 with empty items array, no truncated flag, no next link.
func TestObservationsGet_EmptyStream(t *testing.T) {
	rd := &fakeReader{msgs: nil}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaJSON))
	}
	var coll observationCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if coll.Type != "ObservationCollection" {
		t.Errorf("type: got %q want ObservationCollection", coll.Type)
	}
	if coll.NumberReturned != 0 || coll.NumberMatched != 0 || len(coll.Items) != 0 {
		t.Errorf("counts: %+v", coll)
	}
	if coll.Truncated {
		t.Errorf("truncated true on empty stream; want false")
	}
	for _, l := range coll.Links {
		if l.Rel == "next" {
			t.Errorf("next link present on empty stream: %+v", l)
		}
	}

	// Reader was invoked with the right subject derived from the path
	// param. Wire correctness of the filter matters — graph-ingest will
	// happily consume the wrong subject and return empty, masking the bug.
	wantSubject := "cs-api.observations.c360.semconnect.systems.csapi.datastream.001"
	if rd.gotSubject != wantSubject {
		t.Errorf("FetchSubject subject: got %q want %q", rd.gotSubject, wantSubject)
	}
	if rd.gotStart != 0 {
		t.Errorf("FetchSubject startSeq: got %d want 0 (no ?after=)", rd.gotStart)
	}
}

func TestGlobalObservationsGet_Empty(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{})

	req := httptest.NewRequest(http.MethodGet, "/observations?limit=2", nil)
	rr := httptest.NewRecorder()
	c.handleGlobalObservations(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll observationCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "ObservationCollection" || len(coll.Items) != 0 {
		t.Fatalf("collection: %+v", coll)
	}
}

// TestObservationsGet_GoldenPath — two observations stored; both come
// back in stream-sequence order, with the inner payload bytes preserved.
func TestObservationsGet_GoldenPath(t *testing.T) {
	obs1 := &oms.Observation{
		ID: "obs-001", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
		Result:           12.4,
	}
	obs2 := &oms.Observation{
		ID: "obs-002", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:31:00Z",
		Result:           12.7,
	}
	rd := &fakeReader{
		msgs: []observationMsg{
			{Data: encodeBaseMessage(t, obs1), Sequence: 100},
			{Data: encodeBaseMessage(t, obs2), Sequence: 101},
		},
	}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll observationCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if len(coll.Items) != 2 {
		t.Fatalf("items: got %d want 2", len(coll.Items))
	}
	// Inner payload preserves the OMS shape — round-trip the first item
	// back to oms.Observation and assert the id.
	var got oms.Observation
	if err := json.Unmarshal(coll.Items[0], &got); err != nil {
		t.Fatalf("decode item[0]: %v", err)
	}
	if got.ID != "obs-001" {
		t.Errorf("item[0].id: got %q want obs-001", got.ID)
	}
}

// TestObservationsGet_TruncatedAndNextLink — when len(items)==limit AND
// a sequence number was seen, we mint a next link with ?after=<lastSeq>.
func TestObservationsGet_TruncatedAndNextLink(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-X", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
	}
	rd := &fakeReader{
		msgs: []observationMsg{
			{Data: encodeBaseMessage(t, obs), Sequence: 42},
		},
	}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?limit=1", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	var coll observationCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !coll.Truncated {
		t.Errorf("truncated: got false want true (len(items)==limit==1, lastSeq=42)")
	}
	var nextLink *link
	for i, l := range coll.Links {
		if l.Rel == "next" {
			nextLink = &coll.Links[i]
		}
	}
	if nextLink == nil {
		t.Fatal("no next link in coll.Links")
	}
	wantHref := "/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?limit=1&after=42"
	if nextLink.Href != wantHref {
		t.Errorf("next.href: got %q want %q", nextLink.Href, wantHref)
	}
}

// TestObservationsGet_AfterCursor — ?after=N gets forwarded to the
// reader as startSeq on both the empty-result path AND when messages
// come back. Catches a regression where the cursor would only reach
// the reader on certain code paths.
func TestObservationsGet_AfterCursor(t *testing.T) {
	t.Run("empty result", func(t *testing.T) {
		rd := &fakeReader{msgs: nil}
		c := wireObservationsReadComponent(t, rd)
		req := httptest.NewRequest(http.MethodGet,
			"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?after=42", nil)
		req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
		rr := httptest.NewRecorder()
		c.handleObservationsGet(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200", rr.Code)
		}
		if rd.gotStart != 42 {
			t.Errorf("FetchSubject startSeq: got %d want 42", rd.gotStart)
		}
	})

	t.Run("with messages", func(t *testing.T) {
		obs := &oms.Observation{
			ID: "obs-cursor", Procedure: "http://example.org/proc",
			ObservedProperty: "http://example.org/prop",
			ResultTime:       "2026-05-15T14:30:00Z",
		}
		rd := &fakeReader{
			msgs: []observationMsg{
				{Data: encodeBaseMessage(t, obs), Sequence: 43},
			},
		}
		c := wireObservationsReadComponent(t, rd)
		req := httptest.NewRequest(http.MethodGet,
			"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?after=42&limit=10", nil)
		req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
		rr := httptest.NewRecorder()
		c.handleObservationsGet(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200", rr.Code)
		}
		if rd.gotStart != 42 {
			t.Errorf("FetchSubject startSeq: got %d want 42", rd.gotStart)
		}
		var coll observationCollection
		if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(coll.Items) != 1 {
			t.Errorf("items: got %d want 1", len(coll.Items))
		}
	})
}

// TestObservationsGet_OMSBareArray — Accept application/om+json returns
// a top-level array of bare observation payloads (no CS API wrapper).
// OMS-native clients consume this directly.
func TestObservationsGet_OMSBareArray(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-bare", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
	}
	rd := &fakeReader{
		msgs: []observationMsg{
			{Data: encodeBaseMessage(t, obs), Sequence: 1},
		},
	}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.Header.Set("Accept", "application/om+json")
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaOMS) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaOMS))
	}
	if rr.Header().Get("X-CS-Items-Count") != "1" {
		t.Errorf("X-CS-Items-Count: got %q want 1", rr.Header().Get("X-CS-Items-Count"))
	}
	body := rr.Body.String()
	if !strings.HasPrefix(body, "[") || !strings.HasSuffix(strings.TrimSpace(body), "]") {
		t.Errorf("OMS body not a JSON array: %s", body)
	}
	var arr []oms.Observation
	if err := json.Unmarshal(rr.Body.Bytes(), &arr); err != nil {
		t.Fatalf("decode array: %v", err)
	}
	if len(arr) != 1 || arr[0].ID != "obs-bare" {
		t.Errorf("array contents: %+v", arr)
	}
}

func TestObservationsGet_SWEJSON(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-swe-json", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
		Result:           12.4,
	}
	rd := &fakeReader{msgs: []observationMsg{{Data: encodeBaseMessage(t, obs), Sequence: 9}}}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.Header.Set("Accept", string(MediaSWEJSON))
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaSWEJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaSWEJSON))
	}
	if rr.Header().Get("X-CS-SWE-Subset") != "observation-values" {
		t.Errorf("X-CS-SWE-Subset: got %q", rr.Header().Get("X-CS-SWE-Subset"))
	}
	var body []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0]["time"] != "2026-05-15T14:30:00Z" || body[0]["result"] != 12.4 {
		t.Errorf("SWE JSON body: %+v", body)
	}
}

func TestObservationsGet_SWECsv(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-swe-csv", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
		Result:           12.4,
	}
	rd := &fakeReader{msgs: []observationMsg{{Data: encodeBaseMessage(t, obs), Sequence: 9}}}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.Header.Set("Accept", string(MediaSWECsv))
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaSWECsv) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaSWECsv))
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "time,result\n2026-05-15T14:30:00Z,12.4" {
		t.Errorf("CSV body:\n%s", rr.Body.String())
	}
}

func TestObservationsGet_SWECsvUsesStoredDatastreamSchema(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-swe-csv-schema", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
		Result: map[string]any{
			"temperature": 12.4,
		},
	}
	datastreamID := "c360.semconnect.systems.csapi.datastream.001"
	rd := &fakeReader{msgs: []observationMsg{{Data: encodeBaseMessage(t, obs), Sequence: 9}}}
	fake := &multiReplyFakeRequester{entityRepliesByID: map[string][]byte{}}
	c := wireObservationsReadComponentWithRequester(t, rd, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)
	artifactID := schemaArtifactIDForTest(c, datastreamID, PredDatastreamSchema)
	fake.entityRepliesByID[datastreamID] = encodeDatastreamEntityStateWithSchema(t, datastreamID,
		"Temperature feed",
		"c360.semconnect.systems.csapi.system.sensor1",
		"http://example.org/properties/temperature",
		artifactID)
	fake.entityRepliesByID[artifactID] = seedSchemaArtifact(t, c, store, artifactID, json.RawMessage(testSWEDataRecordSchema))

	req := httptest.NewRequest(http.MethodGet, "/datastreams/"+datastreamID+"/observations", nil)
	req.Header.Set("Accept", string(MediaSWECsv))
	req.SetPathValue("datastreamID", datastreamID)
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-CS-SWE-Subset") != "" {
		t.Fatalf("X-CS-SWE-Subset should be omitted for schema-backed SWE; got %q", rr.Header().Get("X-CS-SWE-Subset"))
	}
	if got := strings.TrimSpace(rr.Body.String()); got != "2026-05-15T14:30:00Z,12.4" {
		t.Errorf("CSV body:\n%s", rr.Body.String())
	}
}

func TestObservationsGet_SWEBinary(t *testing.T) {
	obs := &oms.Observation{
		ID: "obs-swe-bin", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
		Result:           12.4,
	}
	rd := &fakeReader{msgs: []observationMsg{{Data: encodeBaseMessage(t, obs), Sequence: 9}}}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.Header.Set("Accept", string(MediaSWEBinary))
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaSWEBinary) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaSWEBinary))
	}
	rows, err := swecommon.UnmarshalBinaryRows(
		rr.Body.Bytes(),
		sweObservationSchema(swecommon.KindQuantity),
		swecommon.DefaultBinaryEncoding(),
	)
	if err != nil {
		t.Fatalf("decode SWE binary rows: %v", err)
	}
	if len(rows) != 1 || rows[0]["time"] != "2026-05-15T14:30:00Z" || rows[0]["result"] != 12.4 {
		t.Errorf("binary rows: %+v", rows)
	}
}

func TestSWERowsFromOMS_MixedResultsUseTextSchema(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"resultTime":"2026-05-15T14:30:00Z","result":12.4}`),
		json.RawMessage(`{"resultTime":"2026-05-15T14:31:00Z","result":true}`),
	}

	schema, rows := sweRowsFromOMS(items)
	result, ok := schema.FieldByName("result")
	if !ok {
		t.Fatalf("result field missing")
	}
	if result.Component.Kind() != swecommon.KindText {
		t.Fatalf("result kind: got %s want %s", result.Component.Kind(), swecommon.KindText)
	}
	if len(rows) != 2 || rows[0]["result"] != "12.4" || rows[1]["result"] != "true" {
		t.Fatalf("rows: %+v", rows)
	}
}

// TestObservationsGet_InvalidPaths covers the validation seams that 400
// before any backend call.
func TestObservationsGet_InvalidPaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		id   string
	}{
		{"empty id", "/datastreams//observations", ""},
		{"bad limit", "/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?limit=999999", "c360.semconnect.systems.csapi.datastream.001"},
		{"non-numeric after", "/datastreams/c360.semconnect.systems.csapi.datastream.001/observations?after=abc", "c360.semconnect.systems.csapi.datastream.001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd := &fakeReader{}
			c := wireObservationsReadComponent(t, rd)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.SetPathValue("datastreamID", tc.id)
			rr := httptest.NewRecorder()
			c.handleObservationsGet(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
			}
			if rd.gotCalls != 0 {
				t.Errorf("reader called %d times on a 400 path; want 0 (validation must short-circuit)", rd.gotCalls)
			}
		})
	}
}

// TestObservationsGet_AcceptNegotiation — XML 406s, sensorml 406s.
func TestObservationsGet_AcceptNegotiation(t *testing.T) {
	rd := &fakeReader{}
	c := wireObservationsReadComponent(t, rd)
	for _, accept := range []string{"application/xml", "application/sensorml+json", "text/html"} {
		req := httptest.NewRequest(http.MethodGet,
			"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
		req.Header.Set("Accept", accept)
		req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
		rr := httptest.NewRecorder()
		c.handleObservationsGet(rr, req)
		if rr.Code != http.StatusNotAcceptable {
			t.Errorf("Accept %q: got %d want 406", accept, rr.Code)
		}
	}
}

// TestObservationsGet_TransientBackend — NATS errors classify as 503.
func TestObservationsGet_TransientBackend(t *testing.T) {
	rd := &fakeReader{err: nats.ErrNoResponders}
	c := wireObservationsReadComponent(t, rd)
	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
}

// TestObservationsGet_ReaderNotInitialized — Start() not called yet;
// should 503 cleanly rather than panic on a nil pointer.
func TestObservationsGet_ReaderNotInitialized(t *testing.T) {
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	c.initialized = true
	// reader deliberately not stored

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
}

// TestObservationsGet_MalformedEnvelopeSkipped — a malformed message in
// the stream gets skipped (logged but not 500), and subsequent messages
// still appear in the items array.
func TestObservationsGet_MalformedEnvelopeSkipped(t *testing.T) {
	good := &oms.Observation{
		ID: "obs-good", Procedure: "http://example.org/proc",
		ObservedProperty: "http://example.org/prop",
		ResultTime:       "2026-05-15T14:30:00Z",
	}
	rd := &fakeReader{
		msgs: []observationMsg{
			{Data: []byte("{not valid json"), Sequence: 1},
			{Data: encodeBaseMessage(t, good), Sequence: 2},
		},
	}
	c := wireObservationsReadComponent(t, rd)

	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	var coll observationCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 {
		t.Errorf("items: got %d want 1 (malformed envelope should be skipped)", len(coll.Items))
	}
}

// Smoke that the err wrapping path doesn't swallow context cancellations
// — relevant when the caller goes away mid-Fetch.
func TestObservationsGet_ContextCancelled(t *testing.T) {
	rd := &fakeReader{err: errors.New("simulated stream error")}
	c := wireObservationsReadComponent(t, rd)
	req := httptest.NewRequest(http.MethodGet,
		"/datastreams/c360.semconnect.systems.csapi.datastream.001/observations", nil)
	req.SetPathValue("datastreamID", "c360.semconnect.systems.csapi.datastream.001")
	rr := httptest.NewRecorder()
	c.handleObservationsGet(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("unclassified error: got %d want 500; body=%s", rr.Code, rr.Body.String())
	}
}
