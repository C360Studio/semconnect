// Stage 55 — Feasibility handler tests.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

const testFeasibilityID = "c360.semconnect.systems.csapi.feasibility.ptz001"

func feasibilityState(t *testing.T) []byte {
	t.Helper()
	return encodeEntityState(t, graph.EntityState{
		ID: testFeasibilityID,
		Triples: []message.Triple{
			{Subject: testFeasibilityID, Predicate: sensorml.PredType, Object: FeasibilityTypeIRI},
			{Subject: testFeasibilityID, Predicate: PredFeasibilityControlStream, Object: testControlStreamID, Datatype: message.EntityReferenceDatatype},
			{Subject: testFeasibilityID, Predicate: predFeasibilityStatus, Object: "completed"},
			{Subject: testFeasibilityID, Predicate: predFeasibilityParams, Object: `{"pan":10}`},
			{Subject: testFeasibilityID, Predicate: predFeasibilityResult, Object: `{"feasible":true}`},
		},
	})
}

func TestHandleFeasibilities_GoldenPath(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testFeasibilityID}),
		entityRepliesByID: map[string][]byte{
			testFeasibilityID: feasibilityState(t),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/feasibility", nil)
	rr := httptest.NewRecorder()
	c.handleFeasibilities(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll feasibilityCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 {
		t.Fatalf("items: got %d want 1", len(coll.Items))
	}
	got := coll.Items[0]
	if got.ID != testFeasibilityID || got.Type != "Feasibility" || got.ControlStreamID != testControlStreamID {
		t.Errorf("feasibility item: %+v", got)
	}
	if string(got.Params) != `{"pan":10}` || string(got.Result) != `{"feasible":true}` {
		t.Errorf("payloads: params=%s result=%s", got.Params, got.Result)
	}
}

func TestHandleControlStreamFeasibility_UsesSingularNormativePath(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.feasibility.other"
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: FeasibilityTypeIRI},
			{Subject: otherID, Predicate: PredFeasibilityControlStream, Object: "c360.semconnect.systems.csapi.controlstream.other", Datatype: message.EntityReferenceDatatype},
			{Subject: otherID, Predicate: predFeasibilityStatus, Object: "rejected"},
		},
	})
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testFeasibilityID, otherID}),
		entityRepliesByID: map[string][]byte{
			testControlStreamID: controlStreamState(t),
			testFeasibilityID:   feasibilityState(t),
			otherID:             otherState,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/controlstream/"+testControlStreamID+"/feasibility?limit=2", nil)
	req.SetPathValue("id", testControlStreamID)
	rr := httptest.NewRecorder()
	c.handleControlStreamFeasibility(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll feasibilityCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testFeasibilityID {
		t.Fatalf("items: %+v", coll.Items)
	}
	if len(coll.Links) != 1 || coll.Links[0].Href != "/controlstream/"+testControlStreamID+"/feasibility" {
		t.Fatalf("self link: %+v", coll.Links)
	}
}

func TestHandleFeasibility_JSON(t *testing.T) {
	fake := &fakeRequester{
		reply:  feasibilityState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/feasibility/"+testFeasibilityID, nil)
	req.SetPathValue("id", testFeasibilityID)
	rr := httptest.NewRecorder()
	c.handleFeasibility(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got feasibility
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != testFeasibilityID || got.Status != "completed" {
		t.Errorf("feasibility: %+v", got)
	}
	if got.ControlStream == nil || got.ControlStream.Href != "/controlstreams/"+testControlStreamID {
		t.Errorf("controlstream link: %+v", got.ControlStream)
	}
}

func TestHandleFeasibilityStatusAndResult(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		call func(*Component, http.ResponseWriter, *http.Request)
		want string
	}{
		{
			name: "status",
			path: "/feasibility/" + testFeasibilityID + "/status",
			call: (*Component).handleFeasibilityStatus,
			want: `"status":"completed"`,
		},
		{
			name: "result",
			path: "/feasibility/" + testFeasibilityID + "/result",
			call: (*Component).handleFeasibilityResult,
			want: `"feasible":true`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRequester{
				reply:  feasibilityState(t),
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.SetPathValue("id", testFeasibilityID)
			rr := httptest.NewRecorder()
			tc.call(c, rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tc.want) {
				t.Fatalf("body %s does not contain %s", rr.Body.String(), tc.want)
			}
			if !strings.Contains(rr.Body.String(), `"items"`) {
				t.Fatalf("body missing items collection: %s", rr.Body.String())
			}
		})
	}
}

func TestHandleFeasibilityPost_PublishesTriples(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 4),
	}
	c := newTestComponent(t, fake)
	body := []byte(`{"controlstream@id":"` + testControlStreamID + `","status":"completed","params":{"pan":10},"result":{"feasible":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/feasibility", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleFeasibilityPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/feasibility/"+c.cfg.FeasibilityIDPrefix+".") {
		t.Fatalf("Location: %q", loc)
	}
	if !bytes.Contains(fake.gotBody, []byte(PredFeasibilityControlStream)) ||
		!bytes.Contains(fake.gotBody, []byte(predFeasibilityParams)) ||
		!bytes.Contains(fake.gotBody, []byte(predFeasibilityResult)) {
		t.Fatalf("published triples missing feasibility predicates: %s", fake.gotBody)
	}
}
