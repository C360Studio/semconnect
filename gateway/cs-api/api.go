package csapi

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

// openAPIYAML is the served OAS3 contract. Embedded at build time so a
// deploy is one binary — no separate spec-file deploy step. Source-of-
// truth is `api/openapi.yaml`; `api/upstream/` is the unmodified OGC
// vendored snapshot we adapt from (see api/upstream/README.md).
//
//go:embed openapi.yaml
var openAPIYAML []byte

// openAPIJSON is the JSON serialization of openAPIYAML, computed once
// at process start (api.go init() materializes it via sigs.k8s.io/yaml,
// which is the canonical YAML→JSON converter for OpenAPI tooling and
// preserves null/bool/number types correctly across the conversion).
// Holding both lets the handler return either without per-request
// conversion cost.
var openAPIJSON []byte

func init() {
	// Convert the embedded YAML to JSON once. We accept the panic-on-
	// error semantics here because (a) the YAML is checked in and
	// validated in CI before this code can run with a broken spec,
	// and (b) a malformed embedded OAS is a build-time bug that
	// should fail loudly at process start, not at first request.
	//
	// gopkg.in/yaml.v3 deserializes into Go-native types (map[string]any,
	// []any, primitives) that encoding/json marshals correctly — no
	// non-string-key OAS3 edge cases to worry about.
	var generic any
	if err := yaml.Unmarshal(openAPIYAML, &generic); err != nil {
		panic("cs-api: api.go: malformed embedded openapi.yaml: " + err.Error())
	}
	pretty, mErr := json.MarshalIndent(generic, "", "  ")
	if mErr != nil {
		panic("cs-api: api.go: marshal of OAS3 JSON failed: " + mErr.Error())
	}
	openAPIJSON = append(pretty, '\n')
}

// handleAPI serves GET /api (and HEAD) — the OAS3 service definition.
// Stage 12.
//
// Default content type is application/vnd.oai.openapi+json;version=3.0
// because most OpenAPI tooling (Swagger UI, Redoc, openapi-generator,
// kin-openapi) prefers JSON. application/json is honored too — clients
// asking for it get the JSON serialization. ?f=yaml or
// application/vnd.oai.openapi;version=3.0 returns the raw embedded YAML.
//
// This handler is what the landing page's `service-desc` link points
// at, which is the unblock for the upstream-ETS
// `landingPageHasApiDefinitionLink` / `apiDefinitionResourceReturnsContent`
// assertions (issue #1 at Botts-Innovative-Research/ets-ogcapi-connectedsystems10).
func (c *Component) handleAPI(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilyAPI)
	if !ok {
		WriteNotAcceptable(w, FamilyAPI)
		return
	}

	var body []byte
	switch media {
	case MediaOAS3YAML:
		body = openAPIYAML
	default:
		// MediaOAS3JSON and MediaJSON both serve the JSON serialization.
		// FamilyAPI orders MediaOAS3JSON first so the default lands here.
		body = openAPIJSON
	}

	w.Header().Set("Content-Type", string(media))
	// Stable hash-stable content; cache-friendly. Operator-facing
	// reverse proxies can layer their own Cache-Control if needed.
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write api response", "err", err)
	}
}
