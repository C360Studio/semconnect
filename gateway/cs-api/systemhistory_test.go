// Stage 26 — System History handler tests.
package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
)

func TestHandleSystemHistory_CurrentOnlyCollection(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001/history", nil)
	req.SetPathValue("id", "acme.ops.robotics.gcs.drone.001")
	rr := httptest.NewRecorder()
	c.handleSystemHistory(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-CS-History-Current-Only"); got != "true" {
		t.Errorf("X-CS-History-Current-Only: got %q want true", got)
	}
	var coll systemCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.NumberReturned != 1 || len(coll.Items) != 1 {
		t.Fatalf("collection: %+v", coll)
	}
	if coll.Items[0].ID != "acme.ops.robotics.gcs.drone.001" {
		t.Errorf("item id: got %q", coll.Items[0].ID)
	}
	if !strings.Contains(rr.Body.String(), "/history/current") {
		t.Errorf("body missing current revision link: %s", rr.Body.String())
	}
}

func TestHandleSystemHistoryItem_Current(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001/history/current", nil)
	req.SetPathValue("id", "acme.ops.robotics.gcs.drone.001")
	req.SetPathValue("revID", "current")
	rr := httptest.NewRecorder()
	c.handleSystemHistoryItem(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var sys system
	if err := json.Unmarshal(rr.Body.Bytes(), &sys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sys.ID != "acme.ops.robotics.gcs.drone.001" || sys.Label != "ACME Drone 001" {
		t.Errorf("system: %+v", sys)
	}
}

func TestHandleSystemHistoryItem_UnknownRevision404s(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001/history/rev2", nil)
	req.SetPathValue("id", "acme.ops.robotics.gcs.drone.001")
	req.SetPathValue("revID", "rev2")
	rr := httptest.NewRecorder()
	c.handleSystemHistoryItem(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != "" {
		t.Errorf("backend should not be called for unknown revision; got %q", fake.gotSubject)
	}
}
