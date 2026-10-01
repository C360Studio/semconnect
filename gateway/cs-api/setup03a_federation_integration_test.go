//go:build integration

package csapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSetup03AFederatedDatastreamCreate preserves SemConnect's established
// client-supplied six-part identity contract. The graph host has the explicit
// c360.semconnect authority in both the baseline and target reproductions.
// A target rejection is a migration blocker, not permission to narrow HTTP IDs.
func TestSetup03AFederatedDatastreamCreate(t *testing.T) {
	c, _ := setup03AGraph(t)
	const id = "foreign.remote.systems.csapi.datastream.setup03a-reference"
	body := []byte(`{"id":"` + id + `","name":"Federated reference","system":"c360.semconnect.systems.csapi.system.absent","observedProperty":"https://example.test/temperature"}`)
	request := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	request.Header.Set("Content-Type", string(MediaJSON))
	response := httptest.NewRecorder()
	c.handleDatastreamPost(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("federated six-part ID must remain HTTP 201: got %d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Location") != "/datastreams/"+id {
		t.Fatalf("federated identity was rewritten: %s", response.Header().Get("Location"))
	}
	exact, err := c.fetchEntityExact(t.Context(), id)
	if err != nil || exact.KVRevision == 0 || exact.Entity.ID != id {
		t.Fatalf("federated authority missing after create: %+v %v", exact, err)
	}
}
