// cs-graph-backend composes SemConnect's graph processors with its consumer
// payload registry. It intentionally remains a SemStreams binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	csapi "github.com/c360studio/semconnect/gateway/cs-api"
	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/composition"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/c360studio/semstreams/payloadregistry"
	semtypes "github.com/c360studio/semstreams/pkg/types"
	graphindex "github.com/c360studio/semstreams/processor/graph-index"
	graphspatial "github.com/c360studio/semstreams/processor/graph-index-spatial"
	graphtemporal "github.com/c360studio/semstreams/processor/graph-index-temporal"
	graphingest "github.com/c360studio/semstreams/processor/graph-ingest"
	graphquery "github.com/c360studio/semstreams/processor/graph-query"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/c360studio/semstreams/types"
	"github.com/c360studio/semstreams/vocabulary/builtins"
	"github.com/nats-io/nats.go/jetstream"
)

const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cs-graph-backend:", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	path := flag.String("config", "", "path to graph backend JSON configuration")
	consumerPath := flag.String("cs-api-config", "", "gateway JSON configuration used for consumer type contracts")
	validate := flag.Bool("validate", false, "validate the declared composition without NATS")
	identity := flag.String("provision-identity", "", "atomically provision this explicit platform identifier and exit")
	flag.Parse()
	if *path == "" {
		return errors.New("-config is required")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	consumerConfig, err := loadConsumerConfig(*consumerPath)
	if err != nil {
		return err
	}
	cfg, err := loadBackendConfig(*path)
	if err != nil {
		return err
	}
	registry, err := backendRegistry()
	if err != nil {
		return err
	}
	if err := validateComposition(registry, cfg); err != nil {
		return err
	}
	if *validate {
		return nil
	}

	// Signals request controlled shutdown; runtime authority stays live through
	// StopAll, the config watcher join, and the final NATS drain.
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	bootCtx, stopSignals := signal.NotifyContext(runtimeCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	b, err := newBackend(cfg, registry, consumerConfig, logger)
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		runErr = errors.Join(runErr, b.resources.stop(stopCtx))
	}()
	if *identity != "" {
		ctx, cancel := context.WithTimeout(bootCtx, shutdownTimeout)
		defer cancel()
		if err := b.client.Connect(ctx); err != nil {
			return fmt.Errorf("connect for identity provisioning: %w", err)
		}
		return provisionIdentity(ctx, b.client, cfg, *identity)
	}
	if err := startWithAbort(runtimeCtx, bootCtx, cancelRuntime, b.start); err != nil {
		return err
	}
	logger.Info("SemConnect graph backend ready", "org", b.effective.Platform.Org, "platform", b.effective.Platform.ID)
	<-bootCtx.Done()
	return nil
}

func loadBackendConfig(path string) (*config.Config, error) {
	builtins.Register()
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backend configuration: %w", err)
	}
	cfg, err := config.NewLoader().LoadFromBytes(body)
	if err != nil {
		return nil, fmt.Errorf("load backend configuration: %w", err)
	}
	return cfg, nil
}

func backendRegistry() (*component.Registry, error) {
	builtins.Register()
	registry := component.NewRegistry()
	for _, register := range []func(*component.Registry) error{
		graphingest.Register, graphindex.Register, graphspatial.Register,
		graphtemporal.Register, graphquery.Register, objectstore.Register,
	} {
		if err := register(registry); err != nil {
			return nil, fmt.Errorf("register graph component: %w", err)
		}
	}
	return registry, nil
}

func validateComposition(registry *component.Registry, cfg *config.Config) error {
	result, err := composition.Validate(registry, cfg)
	if err != nil {
		return fmt.Errorf("validate graph composition: %w", err)
	}
	var failures []error
	for _, finding := range result.Errors {
		failures = append(failures, fmt.Errorf("%s on %s/%s: %s", finding.Type, finding.Component, finding.Port, finding.Message))
	}
	return errors.Join(failures...)
}

func loadConsumerConfig(path string) (csapi.Config, error) {
	cfg := csapi.DefaultConfig()
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read CS API configuration: %w", err)
		}
		if err := json.Unmarshal(body, &cfg); err != nil {
			return cfg, fmt.Errorf("decode CS API configuration: %w", err)
		}
	}
	cfg.ApplyDefaults()
	return cfg, cfg.Validate()
}

type backend struct {
	consumerConfig csapi.Config
	cfg            *config.Config
	effective      *config.Config
	registry       *component.Registry
	client         *natsclient.Client
	logger         *slog.Logger
	metrics        *metric.MetricsRegistry
	resources      backendResources
}

func newBackend(cfg *config.Config, registry *component.Registry, consumerConfig csapi.Config, logger *slog.Logger) (*backend, error) {
	if cfg == nil || registry == nil || logger == nil {
		return nil, errors.New("backend configuration, registry, and logger are required")
	}
	if err := validateComposition(registry, cfg); err != nil {
		return nil, err
	}
	metrics := metric.NewMetricsRegistry()
	urls := strings.Join(cfg.NATS.URLs, ",")
	if env := os.Getenv("SEMSTREAMS_NATS_URLS"); env != "" {
		urls = env
	}
	client, err := natsclient.NewClient(urls, natsclient.WithLogger(logger), natsclient.WithMetrics(metrics))
	if err != nil {
		return nil, fmt.Errorf("create NATS client: %w", err)
	}
	b := &backend{cfg: cfg, consumerConfig: consumerConfig, registry: registry, client: client, logger: logger, metrics: metrics}
	b.resources.closeNATS = client.Close
	return b, nil
}

func (b *backend) start(runtimeCtx, bootCtx context.Context) error {
	if runtimeCtx == nil || bootCtx == nil {
		return errors.New("backend start requires runtime and bootstrap contexts")
	}
	connectCtx, cancel := context.WithTimeout(bootCtx, 10*time.Second)
	err := b.client.Connect(connectCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("connect NATS: %w", err)
	}
	cm, err := config.NewConfigManager(b.cfg, b.client, b.logger)
	if err != nil {
		return fmt.Errorf("create configuration manager: %w", err)
	}
	b.resources.stopConfig = cm.Stop
	if err := cm.Start(runtimeCtx); err != nil {
		return fmt.Errorf("start configuration manager: %w", err)
	}
	b.effective = cm.GetConfig().Get()
	if err := bootCtx.Err(); err != nil {
		return err
	}
	if err := validateComposition(b.registry, b.effective); err != nil {
		return fmt.Errorf("effective composition: %w", err)
	}
	streams := config.NewStreamsManager(b.client, b.logger)
	if err := streams.VerifyJetStreamLimits(bootCtx, b.effective); err != nil {
		return fmt.Errorf("verify JetStream limits: %w", err)
	}
	if err := streams.EnsureStreams(bootCtx, b.effective); err != nil {
		return fmt.Errorf("provision streams: %w", err)
	}
	payloads := payloadregistry.New()
	if err := payloadbuiltins.Register(payloads); err != nil {
		return fmt.Errorf("register framework payloads: %w", err)
	}
	if err := csapi.RegisterPayloadsWithConfig(payloads, b.consumerConfig); err != nil {
		return fmt.Errorf("register CS API payloads: %w", err)
	}
	if _, err := service.WireGraphRuntime(bootCtx, b.client, b.logger, payloads.Contracts()...); err != nil {
		return err
	}
	services := service.NewServiceRegistry()
	if err := service.RegisterAll(services); err != nil {
		return fmt.Errorf("register services: %w", err)
	}
	manager := service.NewServiceManager(services)
	b.resources.stopServices = manager.StopAll
	deps := &service.Dependencies{
		NATSClient: b.client, MetricsRegistry: b.metrics, Logger: b.logger, Manager: cm,
		ComponentRegistry: b.registry, PayloadRegistry: payloads,
		Platform: types.PlatformMeta{Org: b.effective.GetOrg(), Platform: b.effective.GetPlatform()},
	}
	if err := manager.ConfigureFromServices(b.effective.Services, deps); err != nil {
		return fmt.Errorf("configure services: %w", err)
	}
	if err := bootCtx.Err(); err != nil {
		return err
	}
	if err := manager.StartAll(runtimeCtx); err != nil {
		return fmt.Errorf("start graph services: %w", err)
	}
	return nil
}

type backendResources struct {
	stopServices func(context.Context) error
	stopConfig   func(time.Duration) error
	closeNATS    func(context.Context) error
}

func (r *backendResources) stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("backend stop requires context")
	}
	var result error
	if r.stopServices != nil {
		result = errors.Join(result, r.stopServices(ctx))
	}
	if r.stopConfig != nil {
		// The frozen config.Manager alone still exposes a duration-based stop.
		// Consume the remaining caller budget; never start a fresh deadline.
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.Join(result, errors.New("configuration manager cleanup requires a bounded context"))
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			result = errors.Join(result, ctx.Err())
		} else {
			result = errors.Join(result, r.stopConfig(remaining))
		}
	}
	if r.closeNATS != nil {
		result = errors.Join(result, r.closeNATS(ctx))
	}
	return result
}

type platformIdentity struct {
	Org  string `json:"org"`
	Stem string `json:"stem"`
	ID   string `json:"id"`
}

func verifyIdentity(body []byte, want platformIdentity) error {
	var actual platformIdentity
	if err := json.Unmarshal(body, &actual); err != nil {
		return fmt.Errorf("decode existing platform identity: %w", err)
	}
	if actual != want {
		return fmt.Errorf("existing platform identity differs from requested authority; refusing overwrite")
	}
	return nil
}

func provisionIdentity(ctx context.Context, client *natsclient.Client, cfg *config.Config, id string) error {
	if err := semtypes.ValidateEntityID(cfg.Platform.Org + "." + id + ".identity.config.record.authority"); err != nil {
		return fmt.Errorf("invalid provisioned platform identifier: %w", err)
	}
	bucket, err := config.BucketName(cfg.Platform.Org, cfg.Platform.ID)
	if err != nil {
		return err
	}
	kv, err := graph.EnsureCatalogBucket(ctx, client, bucket)
	if err != nil {
		return fmt.Errorf("acquire identity bucket: %w", err)
	}
	want := platformIdentity{Org: cfg.Platform.Org, Stem: cfg.Platform.ID, ID: id}
	body, err := json.Marshal(want)
	if err != nil {
		return err
	}
	if _, err := kv.Create(ctx, "platform_identity", body); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("create platform identity: %w", err)
		}
		entry, err := kv.Get(ctx, "platform_identity")
		if err != nil {
			return fmt.Errorf("verify existing platform identity: %w", err)
		}
		return verifyIdentity(entry.Value(), want)
	}
	return nil
}

// During incomplete startup a signal is an abort and must reach owners blocked
// in Start. After successful startup the callback is detached before readiness
// is reported, so the same signal requests an orderly stop with live authority.
func startWithAbort(runtimeCtx, bootCtx context.Context, abort context.CancelFunc, start func(context.Context, context.Context) error) error {
	callbackDone := make(chan struct{})
	stopCallback := context.AfterFunc(bootCtx, func() { abort(); close(callbackDone) })
	err := start(runtimeCtx, bootCtx)
	if !stopCallback() {
		<-callbackDone
	}
	return errors.Join(err, bootCtx.Err())
}
