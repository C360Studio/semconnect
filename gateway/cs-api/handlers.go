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
	conformancePath := join("conformance")
	healthPath := join("health")
	datastreamsPath := join("datastreams")
	datastreamItemPath := join("datastreams/{id}")
	observationsPath := join("datastreams/{datastreamID}/observations")
	areasPath := join("areas")

	mux.Handle("GET "+landingPath, c.middleware(http.HandlerFunc(c.handleLanding)))
	mux.Handle("HEAD "+landingPath, c.middleware(http.HandlerFunc(c.handleLanding)))
	mux.Handle("GET "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystems)))
	mux.Handle("HEAD "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystems)))
	mux.Handle("GET "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystem)))
	mux.Handle("HEAD "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystem)))
	mux.Handle("GET "+conformancePath, c.middleware(http.HandlerFunc(c.handleConformance)))
	mux.Handle("HEAD "+conformancePath, c.middleware(http.HandlerFunc(c.handleConformance)))
	mux.Handle("GET "+healthPath, c.middleware(http.HandlerFunc(c.handleHealth)))
	mux.Handle("HEAD "+healthPath, c.middleware(http.HandlerFunc(c.handleHealth)))
	mux.Handle("POST "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystemPost)))
	// Stage 16 — CS API §7.6 create-replace-delete: PUT, DELETE on item;
	// OPTIONS on both collection + item advertising the Allow header.
	mux.Handle("PUT "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemPut)))
	mux.Handle("DELETE "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemDelete)))
	mux.Handle("OPTIONS "+systemsPath, c.middleware(http.HandlerFunc(c.handleSystemsOptions)))
	mux.Handle("OPTIONS "+systemItemPath, c.middleware(http.HandlerFunc(c.handleSystemOptions)))
	mux.Handle("GET "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreams)))
	mux.Handle("HEAD "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreams)))
	mux.Handle("POST "+datastreamsPath, c.middleware(http.HandlerFunc(c.handleDatastreamPost)))
	mux.Handle("GET "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastream)))
	mux.Handle("HEAD "+datastreamItemPath, c.middleware(http.HandlerFunc(c.handleDatastream)))
	mux.Handle("POST "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsPost)))
	mux.Handle("GET "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsGet)))
	mux.Handle("HEAD "+observationsPath, c.middleware(http.HandlerFunc(c.handleObservationsGet)))
	apiPath := join("api")
	mux.Handle("GET "+apiPath, c.middleware(http.HandlerFunc(c.handleAPI)))
	mux.Handle("HEAD "+apiPath, c.middleware(http.HandlerFunc(c.handleAPI)))
	mux.Handle("GET "+areasPath, c.middleware(http.HandlerFunc(c.handleAreas)))
	mux.Handle("HEAD "+areasPath, c.middleware(http.HandlerFunc(c.handleAreas)))

	c.logger.Debug("HTTP handlers registered",
		"landing", landingPath,
		"systems", systemsPath,
		"system_item", systemItemPath,
		"conformance", conformancePath,
		"health", healthPath,
		"datastreams", datastreamsPath,
		"datastream_item", datastreamItemPath,
		"observations", observationsPath,
		"areas", areasPath)
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
