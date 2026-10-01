//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	csapi "github.com/c360studio/semconnect/gateway/cs-api"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestBackendRealNATSConsumerBirthAndControlledStop(t *testing.T) {
	url := startTestNATS(t)
	cfg := testBackendConfig(t, url)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry, err := backendRegistry()
	if err != nil {
		t.Fatal(err)
	}
	provision, err := natsclient.NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if err := provision.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := provisionIdentity(ctx, provision, cfg, "semconnect"); err != nil {
		t.Fatal(err)
	}
	if err := provisionIdentity(ctx, provision, cfg, "semconnect"); err != nil {
		t.Fatalf("idempotent explicit provision: %v", err)
	}
	if err := provisionIdentity(ctx, provision, cfg, "another-authority"); err == nil {
		t.Fatal("identity overwrite accepted")
	}
	if err := provision.Close(ctx); err != nil {
		t.Fatal(err)
	}

	consumerConfig := csapi.DefaultConfig()
	consumerConfig.SystemIDPrefix = "c360.semconnect.custom.csapi.system"
	b, err := newBackend(cfg, registry, consumerConfig, logger)
	if err != nil {
		t.Fatal(err)
	}
	runtimeCtx, cancelRuntime := context.WithCancel(t.Context())
	defer cancelRuntime()
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := b.resources.stop(stopCtx); err != nil {
				t.Error(err)
			}
		}
	})
	if err := b.start(runtimeCtx, ctx); err != nil {
		t.Fatal(err)
	}
	if b.effective.GetPlatform() != "semconnect" {
		t.Fatalf("provisioned authority lost: %s", b.effective.GetPlatform())
	}
	var managerCfg struct {
		HTTPPort int `json:"http_port"`
	}
	if err := json.Unmarshal(cfg.Services["service-manager"].Config, &managerCfg); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(managerCfg.HTTPPort)+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backend not ready: %d %s", resp.StatusCode, body)
	}

	gateway, err := csapi.New(consumerConfig, b.client, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := gateway.Start(runtimeCtx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := gateway.Stop(stopCtx); err != nil {
			t.Error(err)
		}
	})
	mux := http.NewServeMux()
	gateway.RegisterHTTPHandlers("", mux)
	public := httptest.NewServer(mux)
	defer public.Close()
	payload := []byte(`{"type":"Feature","properties":{"uid":"urn:setup03a:backend","name":"Consumer birth"}}`)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, public.URL+"/systems", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("consumer birth: %d %s", resp.StatusCode, body)
	}
	location := resp.Header.Get("Location")
	if location == "" {
		t.Fatal("created System has no Location")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, public.URL+location, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("created resource not readable: %d %s", resp.StatusCode, body)
	}

	public.Close()
	stopCtx, cancelStop := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelStop()
	if err := gateway.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := b.resources.stop(stopCtx); err != nil {
		t.Fatalf("controlled stop: %v", err)
	}
	stopped = true
	if runtimeCtx.Err() != nil {
		t.Fatal("runtime authority cancelled before joins completed")
	}
	observer, err := nats.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	if _, err := observer.RequestWithContext(ctx, "graph.query.entity", []byte(`{"entity_id":"c360.semconnect.systems.csapi.system.absent"}`)); !errors.Is(err, nats.ErrNoResponders) {
		t.Fatalf("responder survived completed stop: %v", err)
	}
}

func startTestNATS(t *testing.T) string {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS failed readiness")
	}
	t.Cleanup(func() { server.Shutdown(); server.WaitForShutdown() })
	return server.ClientURL()
}

func testBackendConfig(t *testing.T, url string) *config.Config {
	t.Helper()
	cfg, err := loadBackendConfig("../../conformance/compose.semstreams.config.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.NATS.URLs = []string{url}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	manager := cfg.Services["service-manager"]
	manager.Config = json.RawMessage(fmtHTTPConfig(port))
	cfg.Services["service-manager"] = manager
	return cfg
}

func fmtHTTPConfig(port int) string {
	return `{"http_port":` + strconv.Itoa(port) + `,"swagger_ui":false}`
}

func TestBackendFailedStartReleasesOwnedResources(t *testing.T) {
	url := startTestNATS(t)
	cfg := testBackendConfig(t, url)
	occupied, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	serviceConfig := cfg.Services["service-manager"]
	serviceConfig.Config = json.RawMessage(fmtHTTPConfig(occupied.Addr().(*net.TCPAddr).Port))
	cfg.Services["service-manager"] = serviceConfig
	registry, err := backendRegistry()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newBackend(cfg, registry, csapi.DefaultConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	runtimeCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := b.start(runtimeCtx, t.Context()); err == nil {
		t.Error("startup unexpectedly acquired occupied HTTP port")
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelStop()
	if err := b.resources.stop(stopCtx); err != nil {
		t.Fatalf("partial startup cleanup: %v", err)
	}
	if runtimeCtx.Err() != nil {
		t.Fatal("failed-start cleanup replaced runtime authority")
	}
	// This listener belongs to the test, not the failed backend; successful dial
	// verifies bounded cleanup did not close a resource it failed to acquire.
	connection, err := net.DialTimeout("tcp", occupied.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("unrelated listener closed: %v", err)
	}
	connection.Close()
}
