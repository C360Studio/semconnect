package csapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// writeJSONError writes a JSON-shaped error body with the right Content-Type.
// http.Error hard-sets text/plain regardless of payload, so we never use it.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error  string `json:"error"`
		Status int    `json:"status"`
	}{msg, status})
}

// RegisterHTTPHandlers registers the cs-api v0.1 endpoint set on mux under
// prefix. ServiceManager calls this with a per-component prefix; the
// standalone Start() path calls it with "".
//
// Every handler is wrapped by the middleware chain so:
//   - Identity is always populated in the request context
//   - request count + lastActivity update for /metrics + /health
//   - panics are recovered and surfaced as 500
func (c *Component) RegisterHTTPHandlers(prefix string, mux *http.ServeMux) {
	join := func(path string) string {
		p := strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/")
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		return p
	}

	// Go 1.22+ ServeMux supports method-and-path patterns; using them
	// uniformly is cleaner than the per-handler r.Method check Stage 2
	// inherited. ServeMux routes by specificity, so /systems and
	// /systems/{id} don't conflict.
	//
	// landingPath uses the `{$}` end-of-path anchor: GET / would otherwise
	// match every unrouted prefix and shadow 404s for typos like /sytems.
	// `GET /{$}` matches only the bare root.
	landingPath := join("{$}")
	systemsPath := join("systems")
	systemItemPath := join("systems/{id}")
	collectionsPath := join("collections")
	conformancePath := join("conformance")
	healthPath := join("health")
	datastreamsPath := join("datastreams")
	datastreamItemPath := join("datastreams/{id}")
	observationsPath := join("datastreams/{datastreamID}/observations")
	areasPath := join("areas")
	proceduresPath := join("procedures")
	procedureItemPath := join("procedures/{id}")
	deploymentsPath := join("deployments")
	deploymentItemPath := join("deployments/{id}")
	samplingFeaturesPath := join("samplingFeatures")
	samplingFeatureItemPath := join("samplingFeatures/{id}")
	propertiesPath := join("properties")
	propertyItemPath := join("properties/{id}")
	controlStreamsPath := join("controlstreams")
	controlStreamItemPath := join("controlstreams/{id}")
	controlStreamSchemaPath := join("controlstreams/{id}/schema")
	controlStreamCommandsPath := join("controlstreams/{id}/commands")
	systemControlStreamsPath := join("systems/{id}/controlstreams")
	systemHistoryPath := join("systems/{id}/history")
	systemHistoryItemPath := join("systems/{id}/history/{revID}")
	systemEventsPath := join("systemEvents")
	systemEventItemPath := join("systemEvents/{id}")
	systemScopedEventsPath := join("systems/{id}/events")
	systemScopedEventItemPath := join("systems/{id}/events/{eventID}")

	mux.Handle("GET "+landingPath, c.middleware(http.HandlerFunc(c.handleLanding)))
	mux.Handle("HEAD "+landingPath, c.middleware(http.HandlerFunc(c.handleLanding)))
	mux.Handle("GET "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystems)))
	mux.Handle("HEAD "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystems)))
	mux.Handle("GET "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystem)))
	mux.Handle("HEAD "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystem)))
	mux.Handle("GET "+collectionsPath, c.middleware(http.HandlerFunc(c.handleCollections)))
	mux.Handle("HEAD "+collectionsPath, c.middleware(http.HandlerFunc(c.handleCollections)))
	mux.Handle("GET "+conformancePath, c.middleware(http.HandlerFunc(c.handleConformance)))
	mux.Handle("HEAD "+conformancePath, c.middleware(http.HandlerFunc(c.handleConformance)))
	mux.Handle("GET "+healthPath, c.middleware(http.HandlerFunc(c.handleHealth)))
	mux.Handle("HEAD "+healthPath, c.middleware(http.HandlerFunc(c.handleHealth)))
	mux.Handle("POST "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystemPost)))
	// Stage 16 — CS API §7.6 create-replace-delete: PUT, DELETE on item;
	// OPTIONS on both collection + item advertising the Allow header.
	mux.Handle("PUT "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemPut)))
	mux.Handle("DELETE "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemDelete)))
	// Stage 19 — CS API conf/update: PATCH partial-update semantics
	// on the item resource.
	mux.Handle("PATCH "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemPatch)))
	mux.Handle("OPTIONS "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystemsOptions)))
	mux.Handle("OPTIONS "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemOptions)))
	mux.Handle("GET "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreams)))
	mux.Handle("HEAD "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreams)))
	mux.Handle("POST "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreamPost)))
	mux.Handle("GET "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastream)))
	mux.Handle("HEAD "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastream)))
	// Stage 17 — /datastreams CRD parity with Stage 16's /systems set.
	mux.Handle("PUT "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastreamPut)))
	mux.Handle("DELETE "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastreamDelete)))
	mux.Handle("OPTIONS "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreamsOptions)))
	mux.Handle("OPTIONS "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastreamOptions)))
	mux.Handle("POST "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsPost)))
	mux.Handle("GET "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsGet)))
	mux.Handle("HEAD "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsGet)))
	apiPath := join("api")
	mux.Handle("GET "+apiPath, c.middleware(http.HandlerFunc(c.handleAPI)))
	mux.Handle("HEAD "+apiPath, c.middleware(http.HandlerFunc(c.handleAPI)))
	mux.Handle("GET "+areasPath, c.middleware(http.HandlerFunc(c.handleAreas)))
	mux.Handle("HEAD "+areasPath, c.middleware(http.HandlerFunc(c.handleAreas)))
	// Stage 20 — /procedures. Read + POST + OPTIONS only at v0.1;
	// CRD/update verbs intentionally absent (ETS doesn't exercise them
	// on this resource type).
	mux.Handle("GET "+proceduresPath, c.middleware(http.HandlerFunc(c.handleProcedures)))
	mux.Handle("HEAD "+proceduresPath, c.middleware(http.HandlerFunc(c.handleProcedures)))
	mux.Handle("POST "+proceduresPath, c.middleware(http.HandlerFunc(c.handleProcedurePost)))
	mux.Handle("OPTIONS "+proceduresPath, c.middleware(http.HandlerFunc(c.handleProceduresOptions)))
	mux.Handle("GET "+procedureItemPath, c.middleware(http.HandlerFunc(c.handleProcedure)))
	mux.Handle("HEAD "+procedureItemPath, c.middleware(http.HandlerFunc(c.handleProcedure)))
	mux.Handle("OPTIONS "+procedureItemPath, c.middleware(http.HandlerFunc(c.handleProcedureOptions)))
	// Stage 21 — /deployments.
	mux.Handle("GET "+deploymentsPath, c.middleware(http.HandlerFunc(c.handleDeployments)))
	mux.Handle("HEAD "+deploymentsPath, c.middleware(http.HandlerFunc(c.handleDeployments)))
	mux.Handle("POST "+deploymentsPath, c.middleware(http.HandlerFunc(c.handleDeploymentPost)))
	mux.Handle("OPTIONS "+deploymentsPath, c.middleware(http.HandlerFunc(c.handleDeploymentsOptions)))
	mux.Handle("GET "+deploymentItemPath, c.middleware(http.HandlerFunc(c.handleDeployment)))
	mux.Handle("HEAD "+deploymentItemPath, c.middleware(http.HandlerFunc(c.handleDeployment)))
	mux.Handle("OPTIONS "+deploymentItemPath, c.middleware(http.HandlerFunc(c.handleDeploymentOptions)))
	// Stage 22 — /samplingFeatures.
	mux.Handle("GET "+samplingFeaturesPath, c.middleware(http.HandlerFunc(c.handleSamplingFeatures)))
	mux.Handle("HEAD "+samplingFeaturesPath, c.middleware(http.HandlerFunc(c.handleSamplingFeatures)))
	mux.Handle("POST "+samplingFeaturesPath, c.middleware(http.HandlerFunc(c.handleSamplingFeaturePost)))
	mux.Handle("OPTIONS "+samplingFeaturesPath, c.middleware(http.HandlerFunc(c.handleSamplingFeaturesOptions)))
	mux.Handle("GET "+samplingFeatureItemPath, c.middleware(http.HandlerFunc(c.handleSamplingFeature)))
	mux.Handle("HEAD "+samplingFeatureItemPath, c.middleware(http.HandlerFunc(c.handleSamplingFeature)))
	mux.Handle("OPTIONS "+samplingFeatureItemPath, c.middleware(http.HandlerFunc(c.handleSamplingFeatureOptions)))
	// Stage 23 — /properties.
	mux.Handle("GET "+propertiesPath, c.middleware(http.HandlerFunc(c.handleProperties)))
	mux.Handle("HEAD "+propertiesPath, c.middleware(http.HandlerFunc(c.handleProperties)))
	mux.Handle("POST "+propertiesPath, c.middleware(http.HandlerFunc(c.handlePropertyPost)))
	mux.Handle("OPTIONS "+propertiesPath, c.middleware(http.HandlerFunc(c.handlePropertiesOptions)))
	mux.Handle("GET "+propertyItemPath, c.middleware(http.HandlerFunc(c.handleProperty)))
	mux.Handle("HEAD "+propertyItemPath, c.middleware(http.HandlerFunc(c.handleProperty)))
	mux.Handle("OPTIONS "+propertyItemPath, c.middleware(http.HandlerFunc(c.handlePropertyOptions)))
	// Stage 24 — Part 2 /controlstreams read-side plus fixture POST.
	mux.Handle("GET "+controlStreamsPath, c.middleware(http.HandlerFunc(c.handleControlStreams)))
	mux.Handle("HEAD "+controlStreamsPath, c.middleware(http.HandlerFunc(c.handleControlStreams)))
	mux.Handle("POST "+controlStreamsPath, c.middleware(http.HandlerFunc(c.handleControlStreamPost)))
	mux.Handle("OPTIONS "+controlStreamsPath, c.middleware(http.HandlerFunc(c.handleControlStreamsOptions)))
	mux.Handle("GET "+controlStreamItemPath, c.middleware(http.HandlerFunc(c.handleControlStream)))
	mux.Handle("HEAD "+controlStreamItemPath, c.middleware(http.HandlerFunc(c.handleControlStream)))
	mux.Handle("OPTIONS "+controlStreamItemPath, c.middleware(http.HandlerFunc(c.handleControlStreamOptions)))
	mux.Handle("GET "+controlStreamSchemaPath, c.middleware(http.HandlerFunc(c.handleControlStreamSchema)))
	mux.Handle("HEAD "+controlStreamSchemaPath, c.middleware(http.HandlerFunc(c.handleControlStreamSchema)))
	mux.Handle("GET "+controlStreamCommandsPath, c.middleware(http.HandlerFunc(c.handleControlStreamCommands)))
	mux.Handle("HEAD "+controlStreamCommandsPath, c.middleware(http.HandlerFunc(c.handleControlStreamCommands)))
	mux.Handle("GET "+systemControlStreamsPath, c.middleware(http.HandlerFunc(c.handleSystemControlStreams)))
	mux.Handle("HEAD "+systemControlStreamsPath, c.middleware(http.HandlerFunc(c.handleSystemControlStreams)))
	// Stage 26 — OSH-bar System History read-side vendor extension.
	mux.Handle("GET "+systemHistoryPath, c.middleware(http.HandlerFunc(c.handleSystemHistory)))
	mux.Handle("HEAD "+systemHistoryPath, c.middleware(http.HandlerFunc(c.handleSystemHistory)))
	mux.Handle("GET "+systemHistoryItemPath, c.middleware(http.HandlerFunc(c.handleSystemHistoryItem)))
	mux.Handle("HEAD "+systemHistoryItemPath, c.middleware(http.HandlerFunc(c.handleSystemHistoryItem)))
	mux.Handle("OPTIONS "+systemHistoryPath, c.middleware(http.HandlerFunc(c.handleSystemHistoryOptions)))
	mux.Handle("OPTIONS "+systemHistoryItemPath, c.middleware(http.HandlerFunc(c.handleSystemHistoryOptions)))
	// Stage 25 — Part 2 /systemEvents read-side plus fixture POST.
	mux.Handle("GET "+systemEventsPath, c.middleware(http.HandlerFunc(c.handleSystemEvents)))
	mux.Handle("HEAD "+systemEventsPath, c.middleware(http.HandlerFunc(c.handleSystemEvents)))
	mux.Handle("POST "+systemEventsPath, c.middleware(http.HandlerFunc(c.handleSystemEventPost)))
	mux.Handle("OPTIONS "+systemEventsPath, c.middleware(http.HandlerFunc(c.handleSystemEventsOptions)))
	mux.Handle("GET "+systemEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemEvent)))
	mux.Handle("HEAD "+systemEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemEvent)))
	mux.Handle("OPTIONS "+systemEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemEventOptions)))
	mux.Handle("GET "+systemScopedEventsPath, c.middleware(http.HandlerFunc(c.handleSystemScopedEvents)))
	mux.Handle("HEAD "+systemScopedEventsPath, c.middleware(http.HandlerFunc(c.handleSystemScopedEvents)))
	mux.Handle("POST "+systemScopedEventsPath, c.middleware(http.HandlerFunc(c.handleSystemScopedEventPost)))
	mux.Handle("OPTIONS "+systemScopedEventsPath, c.middleware(http.HandlerFunc(c.handleSystemEventsOptions)))
	mux.Handle("GET "+systemScopedEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemScopedEvent)))
	mux.Handle("HEAD "+systemScopedEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemScopedEvent)))
	mux.Handle("OPTIONS "+systemScopedEventItemPath, c.middleware(http.HandlerFunc(c.handleSystemEventOptions)))

	c.logger.Debug("HTTP handlers registered",
		"landing", landingPath,
		"systems", systemsPath,
		"system_item", systemItemPath,
		"collections", collectionsPath,
		"conformance", conformancePath,
		"health", healthPath,
		"datastreams", datastreamsPath,
		"datastream_item", datastreamItemPath,
		"observations", observationsPath,
		"areas", areasPath,
		"procedures", proceduresPath,
		"procedure_item", procedureItemPath,
		"deployments", deploymentsPath,
		"deployment_item", deploymentItemPath,
		"sampling_features", samplingFeaturesPath,
		"sampling_feature_item", samplingFeatureItemPath,
		"properties", propertiesPath,
		"property_item", propertyItemPath,
		"controlstreams", controlStreamsPath,
		"controlstream_item", controlStreamItemPath,
		"controlstream_schema", controlStreamSchemaPath,
		"controlstream_commands", controlStreamCommandsPath,
		"system_controlstreams", systemControlStreamsPath,
		"system_history", systemHistoryPath,
		"system_history_item", systemHistoryItemPath,
		"system_events", systemEventsPath,
		"system_event_item", systemEventItemPath,
		"system_scoped_events", systemScopedEventsPath,
		"system_scoped_event_item", systemScopedEventItemPath)
}

// middleware composes the per-request chain. Order matters:
//   - recover first so even a panic in Identity setup is captured
//   - IdentityMiddleware second so handlers always have a populated Identity
//   - body-limit before counting so oversize 413s still tick the counter
//   - request counting innermost so handler-side rejections (4xx/5xx) count too
func (c *Component) middleware(next http.Handler) http.Handler {
	return c.recoverMiddleware(IdentityMiddleware(c.bodyLimitMiddleware(c.countingMiddleware(next))))
}

// bodyLimitMiddleware caps request body size. The seam lands at Stage 2 so
// Stage 3's POST endpoints inherit the limit without an extra wiring step.
// GET requests are unaffected (no body to limit).
func (c *Component) bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength != 0 {
			r.Body = http.MaxBytesReader(w, r.Body, c.cfg.MaxRequestBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (c *Component) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				c.errs.Add(1)
				c.logger.Error("handler panic", "panic", rec, "path", r.URL.Path)
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// countingMiddleware ticks the request counter for every request that reaches
// the handler chain — including those the handler will reject with 4xx/5xx.
// Health and rate metrics treat this as "load offered to the gateway."
// (writeBackendError tracks the subset that errors at 5xx for /health.)
func (c *Component) countingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.requests.Add(1)
		now := time.Now()
		c.lastActivity.Store(&now)
		next.ServeHTTP(w, r)
	})
}

func (c *Component) handleHealth(w http.ResponseWriter, _ *http.Request) {
	h := c.Health()
	w.Header().Set("Content-Type", string(MediaJSON))
	if !h.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{h.Status})
}
