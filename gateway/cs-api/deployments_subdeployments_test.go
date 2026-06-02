package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
)

func TestBuildDeploymentTriplesFromFeature_PreservesParentRelation(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{})
	parentID := "c360.semconnect.systems.csapi.deployment.parent"
	body := []byte(`{
		"type":"Feature",
		"properties":{
			"uid":"child",
			"name":"Child deployment",
			"parent@id":"` + parentID + `"
		}
	}`)

	_, triples, err := c.buildDeploymentTriplesFromFeature(body)
	if err != nil {
		t.Fatalf("buildDeploymentTriplesFromFeature: %v", err)
	}
	if got, ok := firstStringObject(triples, predDeploymentParent); !ok || got != parentID {
		t.Fatalf("parent relation triple: got %q ok=%v triples=%+v", got, ok, triples)
	}
}

func TestHandleDeploymentSubdeployments_ReturnsChildDeployments(t *testing.T) {
	parentID := "c360.semconnect.systems.csapi.deployment.parent"
	childID := "c360.semconnect.systems.csapi.deployment.child"
	otherID := "c360.semconnect.systems.csapi.deployment.other"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{parentID, childID, otherID}),
		entityRepliesByID: map[string][]byte{
			parentID: encodeDeploymentState(t, parentID, nil),
			childID: encodeDeploymentState(t, childID, []message.Triple{
				{Predicate: sensorml.PredLabel, Object: "Child deployment"},
				{Predicate: predDeploymentParent, Object: parentID},
			}),
			otherID: encodeDeploymentState(t, otherID, nil),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/deployments/"+parentID+"/subdeployments", nil)
	req.SetPathValue("id", parentID)
	rr := httptest.NewRecorder()
	c.handleDeploymentSubdeployments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll deploymentCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != childID {
		t.Fatalf("items: %+v", coll.Items)
	}
	if !hasLink(coll.Items[0].Links, "canonical", "/deployments/"+childID) {
		t.Fatalf("child ref missing canonical link: %+v", coll.Items[0].Links)
	}
	if fake.calls != 3 {
		t.Errorf("requests: got %d want 3 (parent entity + predicate + batch)", fake.calls)
	}
}

func encodeDeploymentState(t *testing.T, id string, extra []message.Triple) []byte {
	t.Helper()
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Predicate: sensorml.PredType, Object: ssnDeployment},
			{Predicate: PredSystemUID, Object: id},
		},
	}
	state.Triples = append(state.Triples, extra...)
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encodeDeploymentState: %v", err)
	}
	return b
}
