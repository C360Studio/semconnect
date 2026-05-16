package csapi

import (
	"encoding/json"
	"net/http"
)

// stageConformanceClasses lists the conformance classes the running binary
// actually implements at this stage. Team Engine reads /conformance to decide
// what to assert, so claiming a class we cannot serialise here would fail
// conformance — claim only what works. The full ADR-S001 §1 v0.1 set lands
// over Stages 3–5 as each encoder is wired:
//
//   - Stage 2:                core + json
//   - Stage 3 (this stage):   + oms (consume on POST /datastreams/{id}/observations)
//   - Stage 4 (single system): + sensorml + json-ld
//   - Stage 5 (spatial):       + geojson
//
// Note on oms at Stage 3: this stage wires *consume* only (decode incoming
// application/om+json). Producing OMS on a GET /datastreams/{id}/observations
// is a Stage-4 follow-up. Team Engine's OMS class exercises both sides; the
// claim widens when the GET handler ships.
var stageConformanceClasses = []string{
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/core",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/conf/oms",
}

type conformanceDeclaration struct {
	ConformsTo []string `json:"conformsTo"`
}

// handleConformance serves GET /conformance. CS API §7.4.
func (c *Component) handleConformance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, ok := Negotiate(r.Header.Get("Accept"), FamilyService); !ok {
		WriteNotAcceptable(w, FamilyService)
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(conformanceDeclaration{ConformsTo: stageConformanceClasses})
}
