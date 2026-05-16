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
//   - Stage 3:                + oms (consume on POST /datastreams/{id}/observations)
//   - Stage 4:                + sensorml + json-ld
//   - Stage 5 (this stage):   + geojson (GET /areas)
//
// With Stage 5 merged, the running /conformance declaration aligns with
// ADR-S001 §1's full v0.1 claim. Stage 6 wires the OGC Team Engine
// conformance harness in CI to validate each claim.
//
// Note on sensorml at Stage 4: GET /systems/{id} produces SensorML via a
// lossy triple→sensorml reverse mapping (see gateway/cs-api/sensorml.go for
// what is preserved). Some Team Engine assertions will need the lossy-by-
// design fields documented in the response (X-CS-Reconstructed-Lossy
// header) to pass.
//
// Note on geojson at Stage 5: GET /areas returns Features with
// geometry=null because the framework's SpatialResult only carries entity
// IDs, not their indexed points. RFC 7946 §3.2 permits null geometry, but
// clients needing coordinates must drill via GET /systems/{id}.
var stageConformanceClasses = []string{
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/core",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/conf/oms",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/sensorml",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json-ld",
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/geojson",
}

type conformanceDeclaration struct {
	ConformsTo []string `json:"conformsTo"`
}

// handleConformance serves GET /conformance. CS API §7.4. Method enforced
// by the ServeMux pattern.
func (c *Component) handleConformance(w http.ResponseWriter, r *http.Request) {
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
