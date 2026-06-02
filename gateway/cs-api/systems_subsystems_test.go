package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

func TestBuildSystemTriplesFromFeature_PreservesParentRelation(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{})
	parentID := "c360.semconnect.systems.csapi.system.parent"
	body := []byte(`{
		"type":"Feature",
		"properties":{
			"uid":"child",
			"name":"Child system",
			"parent@id":"` + parentID + `"
		}
	}`)

	_, triples, err := c.buildSystemTriplesFromFeature(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromFeature: %v", err)
	}
	if got, ok := firstStringObject(triples, sensorml.PredIsHostedBy); !ok || got != parentID {
		t.Fatalf("parent relation triple: got %q ok=%v triples=%+v", got, ok, triples)
	}
}

func TestHandleSystemSubsystems_ReturnsHostedSystems(t *testing.T) {
	parentID := "c360.semconnect.systems.csapi.system.parent"
	childID := "c360.semconnect.systems.csapi.system.child"
	otherID := "c360.semconnect.systems.csapi.system.other"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{parentID, childID, otherID}),
		entityRepliesByID: map[string][]byte{
			parentID: encodeSystemState(t, parentID, nil),
			childID: encodeSystemState(t, childID, []message.Triple{
				{Predicate: sensorml.PredLabel, Object: "Child system"},
				{Predicate: sensorml.PredIsHostedBy, Object: parentID},
			}),
			otherID: encodeSystemState(t, otherID, nil),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/"+parentID+"/subsystems", nil)
	req.SetPathValue("id", parentID)
	rr := httptest.NewRecorder()
	c.handleSystemSubsystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll systemCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != childID {
		t.Fatalf("items: %+v", coll.Items)
	}
	if coll.Items[0].Name != "Child system" {
		t.Errorf("item name: got %q", coll.Items[0].Name)
	}
	if fake.calls != 3 {
		t.Errorf("requests: got %d want 3 (parent entity + predicate + batch)", fake.calls)
	}
}

func TestHandleSystemSubsystemItem_HasCanonicalAndParentLinks(t *testing.T) {
	parentID := "c360.semconnect.systems.csapi.system.parent"
	childID := "c360.semconnect.systems.csapi.system.child"
	fake := &multiReplyFakeRequester{
		entityRepliesByID: map[string][]byte{
			parentID: encodeSystemState(t, parentID, nil),
			childID: encodeSystemState(t, childID, []message.Triple{
				{Predicate: sensorml.PredLabel, Object: "Child system"},
				{Predicate: sensorml.PredIsHostedBy, Object: parentID},
			}),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/"+parentID+"/subsystems/"+childID, nil)
	req.SetPathValue("id", parentID)
	req.SetPathValue("subsystemID", childID)
	rr := httptest.NewRecorder()
	c.handleSystemSubsystem(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var sys system
	if err := json.Unmarshal(rr.Body.Bytes(), &sys); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sys.ID != childID || sys.Type != "System" {
		t.Fatalf("system identity: %+v", sys)
	}
	if !hasLink(sys.Links, "canonical", "/systems/"+childID) {
		t.Fatalf("missing canonical link: %+v", sys.Links)
	}
	if !hasLink(sys.Links, "parent", "/systems/"+parentID) {
		t.Fatalf("missing parent link: %+v", sys.Links)
	}
}

func encodeSystemState(t *testing.T, id string, extra []message.Triple) []byte {
	t.Helper()
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Predicate: PredSystemUID, Object: id},
		},
	}
	state.Triples = append(state.Triples, extra...)
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encodeSystemState: %v", err)
	}
	return b
}

func hasLink(links []link, rel, hrefPart string) bool {
	for _, l := range links {
		if l.Rel == rel && strings.Contains(l.Href, hrefPart) {
			return true
		}
	}
	return false
}
