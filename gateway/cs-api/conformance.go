package csapi

import (
	"encoding/json"
	"net/http"
)

// stageConformanceClasses lists the conformance classes the running binary
// actually implements at this stage. Team Engine reads /conformance to decide
// what to assert, so claiming a class we cannot serialise here would fail
// conformance — claim only what works. The full ADR-S001 §1 v0.1 set lands
// over Stages 3–7 as each encoder / endpoint is wired:
//
//   - Stage 2:                core + json
//   - Stage 3:                + oms (consume on POST /datastreams/{id}/observations)
//   - Stage 4:                + sensorml + json-ld
//   - Stage 5:                + geojson (GET /areas)
//   - Stage 7:                + OGC API Common Part 1 Core (URI prepended
//     below) — the CS API Core conformance class implicitly inherits from
//     Common Core, but the Botts ETS asserts the Common Core URI is named
//     explicitly in the declaration. Stage 7 also lands GET / and the
//     ?f= negotiation override that Common Core requires.
//   - Stage 12 (this stage): + oas30 — cs-api now ships an OpenAPI 3.0
//     service definition at GET /api (see gateway/cs-api/api.go and
//     gateway/cs-api/openapi.yaml). The landing page gains a service-desc
//     link pointing at /api, satisfying OGC API Common Part 1 §7.4.1
//     Table 4 ("REQUIRED if oas30 or html is declared").
//
// Stage 6 wires the OGC Team Engine conformance harness in CI; Stage 7
// closes the first-run gaps it surfaced. The Common Core URI is FIRST in
// the list to make the inheritance chain readable to humans grepping the
// /conformance response.
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
//
// Note on Common Core at Stage 7 → Stage 12 update: the OAS30 conformance
// class (.../ogcapi-common-1/1.0/conf/oas30) was originally NOT declared
// because we did not ship an OpenAPI definition. Stage 12 ships one
// (gateway/cs-api/openapi.yaml served at GET /api), so the claim is now
// honest. This also unblocks the upstream-ETS
// landingPageHasApiDefinitionLink / apiDefinitionResourceReturnsContent
// assertions — see docs/upstream-asks/botts-ets-api-definition-unconditional.md
// (issue #1 at Botts-Innovative-Research/ets-ogcapi-connectedsystems10).
var stageConformanceClasses = []string{
	"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/core",
	"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/json",
	"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/oas30",
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
	if _, ok := NegotiateRequest(r, FamilyService); !ok {
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
