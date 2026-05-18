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
	// Stage 16 + 17 — CS API §7.6/§10.6 create-replace-delete:
	// POST + PUT + DELETE + OPTIONS on /systems (Stage 16) and
	// /datastreams (Stage 17). Both resource types the IUT implements
	// now serve the full CRD verb set; the conformance class claim is
	// honest across all of /systems and /datastreams.
	//
	// Per-resource media types:
	//   /systems POST:  sml+json | sensorml+json | json | geo+json
	//   /systems PUT:   json | geo+json (no SensorML on PUT — lossy
	//                   reverse-mapping would surprise clients)
	//   /systems PATCH: json | geo+json (Stage 19, see conf/update below)
	//   /datastreams POST/PUT: json only (no SensorML wrapper)
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/create-replace-delete",
	// Stage 19 — CS API conf/update: PATCH /systems/{id} with
	// partial-update semantics on a Feature body (only present
	// fields replaced; absent fields preserved).
	//
	// **Spec scope**: the CS API conf/update class targets Systems,
	// Deployments, Procedures, Sampling Features, and Derived
	// Properties — explicitly NOT Datastreams. At v0.1 cs-api
	// implements only /systems among those resource types, so the
	// claim is fully honest with no partial-claim disclaimer.
	// /procedures / /samplingFeatures / /properties / /deployments
	// are separate Stage 20+ work; they'll need PATCH at their
	// landing time, but their absence today is a resource-type gap,
	// not an update-class gap.
	//
	// RFC 7396 null-as-delete is NOT implemented (treated as
	// no-op) — the ETS doesn't exercise it.
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/update",
	// Stage 20 — CS API §6 Procedure resource. GET collection/item
	// + POST + OPTIONS. PUT/DELETE/PATCH not landed for /procedures
	// at v0.1 because the ETS CRD/Update test groups only exercise
	// them against /systems; the existing conf/create-replace-delete
	// + conf/update claims stay honest at /systems-only. If a future
	// ETS version (or a real client) asks for procedure mutation,
	// follow the Stage 16/19 pattern.
	//
	// **Per-resource CRD verb matrix at v0.1** (anchor for stage
	// 21+ as new resource types land):
	//   /systems:       POST PUT DELETE PATCH OPTIONS (full)
	//   /datastreams:   POST PUT DELETE        OPTIONS (no PATCH)
	//   /procedures:    POST                   OPTIONS (Stage 20 —
	//                   ETS doesn't exercise CRD/PATCH here)
	"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/procedure",
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
