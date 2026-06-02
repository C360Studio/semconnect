// Stage 28 — OGC API Common Part 2 /collections metadata.
//
// This is discovery only: collection entries point at the canonical CS API
// resource endpoints already implemented by semconnect. We intentionally do
// Stage 47 adds the first narrow /collections/{id}/items facade for the
// SystemEvent resource collection because CS API Part 2 tests exercise that
// collection advertisement explicitly.
package csapi

import (
	"encoding/json"
	"net/http"
)

type collectionsDocument struct {
	Collections []collectionMetadata `json:"collections"`
	Items       []collectionMetadata `json:"items,omitempty"`
	Links       []link               `json:"links"`
}

type collectionMetadata struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ItemType    string `json:"itemType"`
	FeatureType string `json:"featureType,omitempty"`
	Links       []link `json:"links"`
}

func (c *Component) handleCollections(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}

	base := absoluteBase(r)
	collections := buildCollectionsMetadata(base)
	body := collectionsDocument{
		Collections: collections,
		Items:       collections,
		Links: []link{
			{Href: base + "/collections", Rel: "self", Type: string(MediaJSON), Title: "this document"},
		},
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func buildCollectionsMetadata(base string) []collectionMetadata {
	return []collectionMetadata{
		{
			ID:          "all_systems",
			Title:       "All Connected Systems",
			Description: "All systems registered on this server.",
			ItemType:    "feature",
			FeatureType: "sosa:System",
			Links: []link{
				{Href: base + "/systems?f=geojson", Rel: "items", Type: string(MediaGeoJSON), Title: "system resources"},
				{Href: base + "/systems", Rel: "alternate", Type: string(MediaJSON), Title: "system collection"},
			},
		},
		{
			ID:          "all_procedures",
			Title:       "All Procedures",
			Description: "All procedures implemented by systems registered on this server.",
			ItemType:    "feature",
			FeatureType: "sosa:Procedure",
			Links: []link{
				{Href: base + "/procedures?f=geojson", Rel: "items", Type: string(MediaGeoJSON), Title: "procedure resources"},
				{Href: base + "/procedures", Rel: "alternate", Type: string(MediaJSON), Title: "procedure collection"},
			},
		},
		{
			ID:          "all_deployments",
			Title:       "All Deployments",
			Description: "All deployments registered on this server.",
			ItemType:    "feature",
			FeatureType: "sosa:Deployment",
			Links: []link{
				{Href: base + "/deployments?f=geojson", Rel: "items", Type: string(MediaGeoJSON), Title: "deployment resources"},
				{Href: base + "/deployments", Rel: "alternate", Type: string(MediaJSON), Title: "deployment collection"},
			},
		},
		{
			ID:          "all_sampling_features",
			Title:       "All Sampling Features",
			Description: "All sampling features registered on this server.",
			ItemType:    "feature",
			FeatureType: "sosa:Sample",
			Links: []link{
				{Href: base + "/samplingFeatures?f=geojson", Rel: "items", Type: string(MediaGeoJSON), Title: "sampling feature resources"},
				{Href: base + "/samplingFeatures", Rel: "alternate", Type: string(MediaJSON), Title: "sampling feature collection"},
			},
		},
		{
			ID:          "all_properties",
			Title:       "All Observable Properties",
			Description: "All observable property definitions registered on this server.",
			ItemType:    "sosa:Property",
			Links: []link{
				{Href: base + "/properties", Rel: "items", Type: string(MediaJSON), Title: "property resources"},
			},
		},
		{
			ID:          "all_datastreams",
			Title:       "All Datastreams",
			Description: "All datastreams produced by systems registered on this server.",
			ItemType:    "DataStream",
			Links: []link{
				{Href: base + "/datastreams", Rel: "items", Type: string(MediaJSON), Title: "datastream resources"},
			},
		},
		{
			ID:          "all_system_events",
			Title:       "All System Events",
			Description: "All system events registered on this server.",
			ItemType:    "SystemEvent",
			Links: []link{
				{Href: base + "/collections/all_system_events/items", Rel: "items", Type: string(MediaJSON), Title: "system event resources"},
				{Href: base + "/systemEvents", Rel: "alternate", Type: string(MediaJSON), Title: "system event collection"},
			},
		},
	}
}

func (c *Component) handleCollectionItems(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}
	switch r.PathValue("id") {
	case "all_system_events":
		c.handleSystemEvents(w, r)
	default:
		writeJSONError(w, http.StatusNotFound, "collection items facade not available")
	}
}
