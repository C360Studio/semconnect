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
	// RequestWithHeaders is the classified request/reply seam for graph query
	// and mutation calls. Beta.160 typed mutations deliberately pass nil
	// custom headers; their trace/request IDs live in the typed payload and
	// gateway identity remains in the structured mutation audit record.
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

// streamReader is the domain-shaped surface GET observations needs (Stage 11).
// FetchSubject pulls up to `limit` messages from the underlying JetStream
// stream filtered to `subject`, optionally starting after `startSeq` (0 =
// from the beginning). Returns (msgData, sequence) pairs and the highest
// sequence seen — the gateway uses that as the next-link cursor.
//
// The domain-shaped interface (vs exposing raw jetstream.Stream) keeps the
// test seam tight: fakes implement one method, not the full Stream surface.
// The production implementation in jetstreamObservationReader wraps
// OrderedConsumer + FetchNoWait.
type streamReader interface {
	FetchSubject(ctx context.Context, subject string, limit int, startSeq uint64) ([]observationMsg, error)
}

// streamCleaner is the write-side twin of streamReader: DELETE
// /datastreams/{id} uses it to purge all observation messages published
// to the datastream's exact subject after graph triples are removed.
type streamCleaner interface {
	PurgeSubject(ctx context.Context, subject string) error
}

// schemaObjectStore is the ObjectStore surface CS API schema artifacts need.
// Production uses a JetStream ObjectStore; tests provide an in-memory fake.
type schemaObjectStore interface {
	PutBytes(ctx context.Context, name string, data []byte) (*jetstream.ObjectInfo, error)
	GetBytes(ctx context.Context, name string, opts ...jetstream.GetObjectOpt) ([]byte, error)
}

// observationMsg is the minimal per-message tuple FetchSubject returns.
// Data is the raw BaseMessage envelope bytes (the handler unwraps); Sequence
// is the JetStream stream sequence so the gateway can mint a next-link cursor.
type observationMsg struct {
	Data     []byte
	Sequence uint64
	Subject  string
}

// jetstreamObservationReader is the production streamReader: thin wrapper
// over a jetstream.Stream that issues a one-shot ordered consumer per call.
// FetchNoWait returns immediately on an empty filter — important on freshly
// seeded datastreams.
type jetstreamObservationReader struct {
	stream jetstream.Stream
}

type jetstreamObservationCleaner struct {
	stream jetstream.Stream
}

func (c *jetstreamObservationCleaner) PurgeSubject(ctx context.Context, subject string) error {
	return c.stream.Purge(ctx, jetstream.WithPurgeSubject(subject))
}

func (r *jetstreamObservationReader) FetchSubject(
	ctx context.Context,
	subject string,
	limit int,
	startSeq uint64,
) ([]observationMsg, error) {
	cfg := jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{subject},
		ReplayPolicy:   jetstream.ReplayInstantPolicy,
	}
	if startSeq > 0 {
		cfg.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
		// JetStream's by-start-seq is INCLUSIVE — bump by 1 to honor the
		// CS API "strictly after" cursor semantic. Overflow guarded.
		if startSeq < ^uint64(0) {
			cfg.OptStartSeq = startSeq + 1
		} else {
			cfg.OptStartSeq = startSeq
		}
	} else {
		cfg.DeliverPolicy = jetstream.DeliverAllPolicy
	}

	cons, err := r.stream.OrderedConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	batch, err := cons.FetchNoWait(limit)
	if err != nil {
		return nil, err
	}
	out := make([]observationMsg, 0, limit)
	for msg := range batch.Messages() {
		var seq uint64
		if meta, mErr := msg.Metadata(); mErr == nil {
			seq = meta.Sequence.Stream
		}
		out = append(out, observationMsg{Data: msg.Data(), Sequence: seq, Subject: msg.Subject()})
	}
	if berr := batch.Error(); berr != nil {
		return out, berr
	}
	return out, nil
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

	// reader is the JetStream Stream handle used by GET observations
	// (Stage 11). Captured from EnsureStream's return at Start() instead
	// of re-resolving via JetStream().Stream(name) per request — saves
	// a round-trip and proves the stream exists before any reader
	// arrives. Same lifecycle as publisher.
	reader atomic.Pointer[streamReader]

	// cleaner is the JetStream Stream handle used by DELETE /datastreams/{id}
	// to purge observations scoped to that datastream's subject.
	cleaner atomic.Pointer[streamCleaner]

	// schemaArtifacts is the JetStream ObjectStore used by first-class SWE
	// schema artifact entities. Set during Start() after JetStream is healthy.
	schemaArtifacts atomic.Pointer[schemaObjectStore]

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

const (
	graphMutationPortName      = "graph-mutations"
	graphMutationSubjectFamily = "graph.mutation.>"
	graphMutationInterfaceType = "semstreams.graph.mutation"
	graphMutationInterfaceVer  = "v1"
	graphMutationCreateOp      = "entity.create"
	graphMutationReconcileOp   = "entity.reconcile"
	graphMutationDeleteOp      = "entity.delete"
)

func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        componentName,
		Type:        "gateway",
		Description: "OGC API Connected Systems v1.0 HTTP gateway",
		Version:     "0.1.0",
	}
}

func (c *Component) InputPorts() []component.Port {
	// HTTP routes are directly composed by RegisterHTTPHandlers and are not
	// SemStreams flow ports.
	return nil
}

func (c *Component) OutputPorts() []component.Port {
	defs := c.outputPortDefinitions()
	out := make([]component.Port, len(defs))
	for i, d := range defs {
		port, err := d.Resolve(component.DirectionOutput)
		if err != nil {
			// Discoverable has no error return. Every value is validated from
			// Config in New; reaching this point is a programmer error.
			panic(fmt.Sprintf("cs-api: resolve output port %q: %v", d.Name, err))
		}
		out[i] = port
	}
	return out
}

func (c *Component) outputPortDefinitions() []component.PortDefinition {
	return []component.PortDefinition{
		{Name: graphMutationPortName, Required: true, Description: "typed revision-fenced graph mutation family", Config: component.NATSRequestPort{Subject: graphMutationSubjectFamily, Timeout: c.cfg.QueryTimeout.String(), Interface: &component.InterfaceContract{Type: graphMutationInterfaceType, Version: graphMutationInterfaceVer}}},
		{Name: "entity-query", Required: true, Description: "fetch exact entity state and authority revision", Config: component.NATSRequestPort{Subject: "graph.query.entity", Timeout: c.cfg.QueryTimeout.String(), Interface: &component.InterfaceContract{Type: "graph.query", Version: "v1"}}},
		{Name: "batch-query", Required: true, Description: "hydrate collection entity states in chunks", Config: component.NATSRequestPort{Subject: "graph.query.batch", Timeout: c.cfg.QueryTimeout.String(), Interface: &component.InterfaceContract{Type: "graph.query", Version: "v1"}}},
		{Name: "predicate-query", Required: true, Description: "list entities by type", Config: component.NATSRequestPort{Subject: "graph.index.query.predicate", Timeout: c.cfg.QueryTimeout.String()}},
		{Name: "spatial-bounds-query", Required: true, Description: "bbox-filtered entity list", Config: component.NATSRequestPort{Subject: "graph.spatial.query.bounds", Timeout: c.cfg.QueryTimeout.String()}},
		{Name: "spatial-polygon-query", Required: true, Description: "polygon-contained entity list", Config: component.NATSRequestPort{Subject: "graph.spatial.query.polygon", Timeout: c.cfg.QueryTimeout.String()}},
		{Name: "observations", Required: true, Description: "OMS observation stream", Config: component.JetStreamPort{StreamName: c.cfg.ObservationsStream, Subjects: []string{c.cfg.ObservationsSubjectPrefix + ".>"}, Storage: "file", RetentionPolicy: "limits", Replicas: c.cfg.ObservationsReplicas}},
	}
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
	stream, err := c.nats.EnsureStream(ctx, observationStreamConfig(c.cfg))
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: ensure stream %s: %w", c.cfg.ObservationsStream, err)
	}
	js, err := c.nats.JetStream()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: jetstream handle: %w", err)
	}
	schemaStore, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      c.cfg.SchemaArtifactsBucket,
		Description: "cs-api canonical SWE schema artifacts",
		Storage:     jetstream.FileStorage,
		MaxBytes:    c.cfg.SchemaArtifactsMaxBytes,
		Replicas:    c.cfg.SchemaArtifactsReplicas,
	})
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: ensure object store %s: %w", c.cfg.SchemaArtifactsBucket, err)
	}
	// EnsureStream + the publisher/reader handles are intentionally not
	// torn down when a later Start() step fails. EnsureStream is
	// idempotent on its JetStream side, and leaving the handles pre-set
	// means a retry of Start() does no extra round-trips. Operators
	// inspecting via Health() will see "stopped" until Start() runs
	// cleanly to completion.
	var pub streamPublisher = js
	c.publisher.Store(&pub)
	var rd streamReader = &jetstreamObservationReader{stream: stream}
	c.reader.Store(&rd)
	var cleaner streamCleaner = &jetstreamObservationCleaner{stream: stream}
	c.cleaner.Store(&cleaner)
	var artifacts schemaObjectStore = schemaStore
	c.schemaArtifacts.Store(&artifacts)

	if err := c.bindProjectionContracts(ctx); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("cs-api: Start: bind projection contracts: %w", err)
	}

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

func observationStreamConfig(cfg Config) jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:        cfg.ObservationsStream,
		Subjects:    []string{cfg.ObservationsSubjectPrefix + ".>"},
		Description: "cs-api observations published via POST /datastreams/{id}/observations",
		Retention:   jetstream.LimitsPolicy, // facts, not work-queue — multi-consumer
		Storage:     jetstream.FileStorage,
		MaxAge:      cfg.ObservationsMaxAge,
		MaxBytes:    cfg.ObservationsMaxBytes,
		Discard:     jetstream.DiscardOld,
		Replicas:    cfg.ObservationsReplicas,
	}
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
