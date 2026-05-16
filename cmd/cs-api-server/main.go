// cs-api-server is the reference deployment binary for semconnect — the OGC
// API Connected Systems v1.0 HTTP gateway. v0.1 ships GET /systems +
// /conformance + /health.
//
// Usage:
//
//	cs-api-server -config ./cs-api.json
//
// Config shape (all fields optional; ApplyDefaults fills the rest):
//
//	{
//	  "nats_url":         "nats://localhost:4222",
//	  "bind_address":     ":8080",
//	  "query_timeout":    "5s",
//	  "default_list_limit": 100,
//	  "max_list_limit":   1000
//	}
//
// The binary intentionally avoids the full semstreams service.ServiceManager
// at v0.1 — it constructs the cs-api Component directly and runs its standalone
// HTTP server. Embedding under ServiceManager is a follow-up once the surface
// stabilizes past Stage 5.
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
	"syscall"
	"time"

	"github.com/c360studio/semstreams/natsclient"

	csapi "github.com/c360studio/semconnect/gateway/cs-api"
)

const (
	defaultNATSURL   = "nats://localhost:4222"
	shutdownDeadline = 10 * time.Second
	natsConnDeadline = 10 * time.Second
)

// serverConfig is the on-disk shape. It embeds csapi.Config plus the bits the
// binary itself needs (NATS URL, log level) — those are deployment concerns
// the Component itself does not care about.
type serverConfig struct {
	NATSURL  string `json:"nats_url"`
	LogLevel string `json:"log_level"`
	csapi.Config
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "cs-api-server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to JSON config (optional; defaults are sane for local dev)")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := buildLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	// Force standalone mode — this binary owns its HTTP server. A future
	// embedded-under-ServiceManager binary would flip this off.
	cfg.Config.StandaloneServer = true

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	natsCtx, natsCancel := context.WithTimeout(ctx, natsConnDeadline)
	defer natsCancel()
	nc, err := natsclient.NewClient(cfg.NATSURL)
	if err != nil {
		return fmt.Errorf("nats client: %w", err)
	}
	if err := nc.Connect(natsCtx); err != nil {
		return fmt.Errorf("nats connect: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
		defer cancel()
		if err := nc.Close(closeCtx); err != nil {
			logger.Warn("nats close", "err", err)
		}
	}()
	logger.Info("connected to NATS", "url", cfg.NATSURL)

	comp, err := csapi.New(cfg.Config, nc, logger)
	if err != nil {
		return fmt.Errorf("build cs-api component: %w", err)
	}
	if err := comp.Initialize(); err != nil {
		return fmt.Errorf("initialize cs-api: %w", err)
	}
	if err := comp.Start(ctx); err != nil {
		return fmt.Errorf("start cs-api: %w", err)
	}

	<-ctx.Done()
	logger.Info("shutting down", "reason", ctx.Err())
	if err := comp.Stop(shutdownDeadline); err != nil {
		return fmt.Errorf("stop cs-api: %w", err)
	}
	return nil
}

func loadConfig(path string) (serverConfig, error) {
	cfg := serverConfig{
		NATSURL:  defaultNATSURL,
		LogLevel: "info",
		Config:   csapi.DefaultConfig(),
	}
	if path == "" {
		cfg.Config.ApplyDefaults()
		if err := cfg.Config.Validate(); err != nil {
			return cfg, fmt.Errorf("default config invalid: %w", err)
		}
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.NATSURL == "" {
		return cfg, errors.New("nats_url required when config file is supplied")
	}
	cfg.Config.ApplyDefaults()
	if err := cfg.Config.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid cs-api config: %w", err)
	}
	return cfg, nil
}

func buildLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
