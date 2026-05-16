package csapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/gateway"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// natsRequester abstracts the slice of *natsclient.Client the cs-api gateway
// uses, so tests can supply a deterministic mock without standing up NATS.
// *natsclient.Client satisfies this interface — see the framework's
// natsclient/client.go.
//
// Stage 3 adds the publish + JetStream pair: gateways need a way to
// EnsureStream at startup and publish observations with audit headers
// (natsclient.PublishToStream does not expose a headers parameter, so we
// drop down to js.PublishMsg with our own *nats.Msg).
type natsRequester interface {
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
	// RequestWithHeaders is used by Stage 8 mutation handlers (POST /systems,
	// POST /datastreams) so audit headers from X-Forwarded-* propagate onto
	// the NATS request envelope. graph-ingest does not consume the headers
	// today, but a trace tool sniffing the request subject does — and the
	// symmetry with observations.go's audited publish path matters for
	// operator runbooks.
	RequestWithHeaders(ctx context.Context, subject string, data []byte, headers map[string]string, timeout time.Duration) (*nats.Msg, error)
	Status() natsclient.ConnectionStatus
	JetStream() (jetstream.JetStream, error)
	EnsureStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error)
}

// streamPublisher is the narrow surface observations.go needs. *jetstream.JetStream
// from natsclient.Client.JetStream() satisfies it. Tests substitute a fake.
type streamPublisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Component is the cs-api gateway. It implements:
//   - component.Discoverable      (framework discovery)
//   - component.LifecycleComponent (Initialize / Start / Stop)
//   - gateway.Gateway             (RegisterHTTPHandlers)
type Component struct {
	cfg    Config
	nats   natsRequester
	logger *slog.Logger

	mu          sync.RWMutex
	initialized bool
	running     bool
	startTime   time.Time

	httpServer   *http.Server
	httpMux      *http.ServeMux
	httpListener net.Listener

	// publisher is the JetStream handle used by mutation endpoints
	// (observations POST). Set once during Start() after EnsureStream
	// and never reassigned. atomic.Pointer makes the read-only contract
	// self-documenting and survives a future Stop() that drains by
	// nilling the publisher.
	publisher atomic.Pointer[streamPublisher]

	errs         atomic.Int64
	requests     atomic.Int64
	lastActivity atomic.Pointer[time.Time]
}

// Verify interface satisfaction at compile time.
var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
	_ gateway.Gateway              = (*Component)(nil)
)

// New constructs a Component. The constructor is test-friendly: pass a mock
// natsRequester and a nil logger to drive handlers from unit tests.
func New(cfg Config, nats natsRequester, logger *slog.Logger) (*Component, error) {
	if nats == nil {
		return nil, errors.New("cs-api: nats requester required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("cs-api: invalid config: %w", err)
	}
	now := time.Now()
	c := &Component{cfg: cfg, nats: nats, logger: logger.With("component", "cs-api")}
	c.lastActivity.Store(&now)
	return c, nil
}

// ---------------- Discoverable ----------------

const componentName = "cs-api"

func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        componentName,
		Type:        "gateway",
		Description: "OGC API Connected Systems v1.0 HTTP gateway",
		Version:     "0.1.0",
	}
}

func (c *Component) InputPorts() []component.Port {
	defs := []component.PortDefinition{
		{Name: "http-systems", Type: "http", Subject: "/systems", Description: "GET /systems"},
		{Name: "http-conformance", Type: "http", Subject: "/conformance", Description: "GET /conformance"},
	}
	out := make([]component.Port, len(defs))
	for i, d := range defs {
		out[i] = component.BuildPortFromDefinition(d, component.DirectionInput)
	}
	return out
}

func (c *Component) OutputPorts() []component.Port {
	defs := []component.PortDefinition{
		{Name: "predicate-query", Type: "nats-request", Subject: "graph.index.query.predicate", Description: "list entities by rdf:type"},
		{Name: "entity-query", Type: "nats-request", Subject: "graph.query.entity", Description: "fetch entity state by ID for GET /systems/{id}"},
		{Name: "spatial-bounds-query", Type: "nats-request", Subject: "graph.spatial.query.bounds", Description: "bbox-filtered entity list for GET /areas?bbox"},
		{Name: "spatial-polygon-query", Type: "nats-request", Subject: "graph.spatial.query.polygon", Description: "polygon-contained entity list for GET /areas?polygon"},
		{Name: "observations", Type: "jetstream", Subject: c.cfg.ObservationsSubjectPrefix + ".>", StreamName: c.cfg.ObservationsStream, Description: "OMS observations from POST /datastreams/{id}/observations"},
	}
	out := make([]component.Port, len(defs))
	for i, d := range defs {
		out[i] = component.BuildPortFromDefinition(d, component.DirectionOutput)
	}
	return out
}

func (c *Component) ConfigSchema() component.ConfigSchema {
	// v0.1: no rich schema yet — operators read defaults from Go source.
	// Wire `component.GenerateConfigSchema(reflect.TypeOf(Config{}))` once
	// Config carries `schema:"..."` tags.
	return component.ConfigSchema{}
}

func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	running := c.running
	uptime := time.Duration(0)
	if running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}
	c.mu.RUnlock()

	errs := int(c.errs.Load())
	natsStatus := c.nats.Status()
	natsHealthy := natsStatus == natsclient.StatusConnected

	status := "stopped"
	if running {
		if natsHealthy {
			status = "running"
		} else {
			status = "degraded"
		}
	}
	return component.HealthStatus{
		Healthy:    running && natsHealthy && errs == 0,
		LastCheck:  time.Now(),
		ErrorCount: errs,
		Uptime:     uptime,
		Status:     status,
	}
}

func (c *Component) DataFlow() component.FlowMetrics {
	c.mu.RLock()
	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}
	c.mu.RUnlock()

	msgs := c.requests.Load()
	var rate float64
	if uptime > 0 {
		rate = float64(msgs) / uptime.Seconds()
	}
	last := time.Now()
	if p := c.lastActivity.Load(); p != nil {
		last = *p
	}
	return component.FlowMetrics{
		MessagesPerSecond: rate,
		LastActivity:      last,
	}
}

// ---------------- LifecycleComponent ----------------

func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initialized {
		return nil
	}
	if err := c.cfg.Validate(); err != nil {
		return fmt.Errorf("cs-api: Initialize: %w", err)
	}
	c.initialized = true
	c.logger.Info("initialized")
	return nil
}

func (c *Component) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("cs-api: Start: context required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cs-api: Start: context already cancelled: %w", err)
	}

	c.mu.Lock()
	if !c.initialized {
		c.mu.Unlock()
		return errors.New("cs-api: Start: not initialized")
	}
	if c.running {
		c.mu.Unlock()
		return nil
	}

	// Ensure the observations JetStream stream exists + capture a publish
	// handle. Doing this synchronously in Start() means the first POST
	// does not race the stream's creation, and a configuration that
	// cannot reach JetStream surfaces here instead of inside a 503'd
	// handler.
	if _, err := c.nats.EnsureStream(ctx, jetstream.StreamConfig{
		Name:        c.cfg.ObservationsStream,
		Subjects:    []string{c.cfg.ObservationsSubjectPrefix + ".>"},
		Description: "cs-api observations published via POST /datastreams/{id}/observations",
		Retention:   jetstream.LimitsPolicy, // facts, not work-queue — multi-consumer
		Storage:     jetstream.FileStorage,
		MaxAge:      c.cfg.ObservationsMaxAge,
		MaxBytes:    c.cfg.ObservationsMaxBytes, // 0 = unlimited
		Replicas:    c.cfg.ObservationsReplicas,
	}); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: ensure stream %s: %w", c.cfg.ObservationsStream, err)
	}
	js, err := c.nats.JetStream()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: jetstream handle: %w", err)
	}
	// EnsureStream + the publisher handle are intentionally not torn down
	// when a later Start() step fails. EnsureStream is idempotent on its
	// JetStream side, and leaving the publisher pre-set means a retry of
	// Start() does no extra round-trips. Operators inspecting via Health()
	// will see "stopped" until Start() runs cleanly to completion.
	var pub streamPublisher = js
	c.publisher.Store(&pub)

	if c.cfg.StandaloneServer {
		// Bind synchronously so a port conflict / permission error
		// surfaces as a Start() error instead of a silently-orphaned
		// goroutine and a process that looks healthy without a listener.
		listener, err := net.Listen("tcp", c.cfg.BindAddress)
		if err != nil {
			c.mu.Unlock()
			return fmt.Errorf("cs-api: Start: listen %s: %w", c.cfg.BindAddress, err)
		}
		c.httpListener = listener
		c.httpMux = http.NewServeMux()
		c.RegisterHTTPHandlers("", c.httpMux)
		c.httpServer = &http.Server{
			Handler:           c.httpMux,
			ReadHeaderTimeout: c.cfg.ReadHeaderTimeout,
			ReadTimeout:       c.cfg.ReadTimeout,
			WriteTimeout:      c.cfg.WriteTimeout,
			IdleTimeout:       c.cfg.IdleTimeout,
		}
	}

	c.running = true
	c.startTime = time.Now()
	srv := c.httpServer
	listener := c.httpListener
	c.mu.Unlock()

	if srv != nil {
		go func() {
			c.logger.Info("HTTP server listening", "bind", listener.Addr().String())
			if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
				c.logger.Error("HTTP server exited", "err", err)
				c.errs.Add(1)
			}
		}()
	}
	c.logger.Info("started", "standalone", c.cfg.StandaloneServer)
	return nil
}

func (c *Component) Stop(timeout time.Duration) error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	srv := c.httpServer
	c.running = false
	c.mu.Unlock()

	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("cs-api: Stop: %w", err)
		}
	}
	c.logger.Info("stopped")
	return nil
}
