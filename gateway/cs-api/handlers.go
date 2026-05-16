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

	mux.Handle(join("systems"), c.middleware(http.HandlerFunc(c.handleSystems)))
	mux.Handle(join("conformance"), c.middleware(http.HandlerFunc(c.handleConformance)))
	mux.Handle(join("health"), c.middleware(http.HandlerFunc(c.handleHealth)))

	c.logger.Debug("HTTP handlers registered",
		"systems", join("systems"),
		"conformance", join("conformance"),
		"health", join("health"))
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
