package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
)

func TestHandleCollections(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})
	req := httptest.NewRequest(http.MethodGet, "/collections", nil)
	rr := httptest.NewRecorder()
	c.handleCollections(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
	var body collectionsDocument
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Collections) == 0 {
		t.Fatal("collections empty")
	}
	if len(body.Links) == 0 || body.Links[0].Rel != "self" {
		t.Fatalf("self link missing: %+v", body.Links)
	}

	byID := map[string]collectionMetadata{}
	for _, coll := range body.Collections {
		byID[coll.ID] = coll
		for _, l := range coll.Links {
			if !strings.HasPrefix(l.Href, "http://") && !strings.HasPrefix(l.Href, "https://") {
				t.Errorf("collection %s link href is not absolute: %q", coll.ID, l.Href)
			}
		}
	}
	check := map[string]struct {
		itemType    string
		featureType string
	}{
		"all_systems":           {"feature", "sosa:System"},
		"all_procedures":        {"feature", "sosa:Procedure"},
		"all_deployments":       {"feature", "sosa:Deployment"},
		"all_sampling_features": {"feature", "sosa:Sample"},
		"all_properties":        {"sosa:Property", ""},
	}
	for id, want := range check {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("missing collection %s; got ids=%v", id, keysCollections(byID))
		}
		if got.ItemType != want.itemType || got.FeatureType != want.featureType {
			t.Errorf("%s markers: got itemType=%q featureType=%q want itemType=%q featureType=%q",
				id, got.ItemType, got.FeatureType, want.itemType, want.featureType)
		}
	}
}

func TestHandleCollections_HEAD(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})
	req := httptest.NewRequest(http.MethodHead, "/collections", nil)
	rr := httptest.NewRecorder()
	c.handleCollections(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD body should be empty, got %q", rr.Body.String())
	}
}

func TestHandleCollections_FParameter(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})

	req := httptest.NewRequest(http.MethodGet, "/collections?f=json", nil)
	rr := httptest.NewRecorder()
	c.handleCollections(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("f=json status: got %d want 200", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/collections?f=geojson", nil)
	rr = httptest.NewRecorder()
	c.handleCollections(rr, req)
	if rr.Code != http.StatusNotAcceptable {
		t.Fatalf("f=geojson status: got %d want 406", rr.Code)
	}
}

func keysCollections(m map[string]collectionMetadata) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
