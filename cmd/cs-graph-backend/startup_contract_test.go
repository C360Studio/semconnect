//go:build integration

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	csapi "github.com/c360studio/semconnect/gateway/cs-api"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
)

func TestBackendRejectsMissingAndUnknownComposition(t *testing.T) {
	registry, err := backendRegistry()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testBackendConfig(t, "nats://127.0.0.1:4222")
	if _, err := newBackend(nil, registry, csapi.DefaultConfig(), logger); err == nil {
		t.Fatal("nil configuration accepted")
	}
	if _, err := newBackend(cfg, nil, csapi.DefaultConfig(), logger); err == nil {
		t.Fatal("nil registry accepted")
	}
	if _, err := newBackend(cfg, registry, csapi.DefaultConfig(), nil); err == nil {
		t.Fatal("nil logger accepted")
	}
	for name, component := range cfg.Components {
		component.Type = "unregistered-operator-component"
		cfg.Components[name] = component
		break
	}
	if _, err := newBackend(cfg, registry, csapi.DefaultConfig(), logger); err == nil {
		t.Fatal("unregistered configured component accepted")
	}
}

func TestConsumerConfigLoadingRejectsUnreadableAndInvalidAuthority(t *testing.T) {
	if _, err := loadConsumerConfig(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("missing consumer config accepted")
	}
	for _, body := range []string{"{", `{"system_id_prefix":"invalid-prefix"}`, `{"system_id_prefix":"acme.custom.systems.csapi.system"}`} {
		path := filepath.Join(t.TempDir(), "consumer.json")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConsumerConfig(path)
		if strings.Contains(body, "acme.custom") {
			if err != nil || cfg.SystemIDPrefix != "acme.custom.systems.csapi.system" {
				t.Fatalf("custom authority: %+v %v", cfg, err)
			}
		} else if err == nil {
			t.Fatal("invalid consumer config accepted")
		}
	}
	if _, err := loadConsumerConfig(""); err != nil {
		t.Fatal(err)
	}
}

func TestBackendStartupFailuresReleaseAcquiredResources(t *testing.T) {
	for _, name := range []string{"nil runtime", "nil bootstrap", "canceled bootstrap", "canceled runtime", "invalid consumer authority", "unknown service", "invalid stream"} {
		t.Run(name, func(t *testing.T) {
			url := startTestNATS(t)
			cfg := testBackendConfig(t, url)
			consumer := csapi.DefaultConfig()
			if name == "invalid consumer authority" {
				consumer.SystemIDPrefix = "invalid-prefix"
			}
			if name == "unknown service" {
				for key, service := range cfg.Services {
					delete(cfg.Services, key)
					cfg.Services["unregistered-service"] = service
					break
				}
			}
			if name == "invalid stream" {
				// Retain valid composition but ask NATS to reject stream provisioning.
				cfg.Streams = config.StreamConfigs{"INVALID_SUBJECT": {
					Subjects: []string{"invalid..subject"}, MaxAge: "1h", MaxBytes: 1024, Discard: "old",
				}}
			}
			registry, err := backendRegistry()
			if err != nil {
				t.Fatal(err)
			}
			b, err := newBackend(cfg, registry, consumer, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
			defer cancelRuntime()
			bootCtx, cancelBoot := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelBoot()
			if name == "nil runtime" {
				runtimeCtx = nil
			}
			if name == "nil bootstrap" {
				bootCtx = nil
			}
			if name == "canceled bootstrap" {
				cancelBoot()
			}
			if name == "canceled runtime" {
				cancelRuntime()
			}
			if err := b.start(runtimeCtx, bootCtx); err == nil {
				t.Errorf("invalid startup %s accepted", name)
			}
			stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelStop()
			if err := b.resources.stop(stopCtx); err != nil {
				t.Fatalf("partial startup resources did not close: %v", err)
			}
		})
	}
}

func TestIdentityProvisioningRejectsInvalidInputsBeforeMutation(t *testing.T) {
	url := startTestNATS(t)
	cfg := testBackendConfig(t, url)
	client, err := natsclient.NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := provisionIdentity(ctx, client, cfg, "invalid.platform"); err == nil {
		t.Fatal("invalid authority accepted")
	}
	if err := provisionIdentity(ctx, client, cfg, "semconnect"); err == nil {
		t.Fatal("unconnected identity write accepted")
	}
	cfg.Platform.ID = "invalid platform stem"
	if err := provisionIdentity(ctx, client, cfg, "semconnect"); err == nil {
		t.Fatal("invalid bucket authority accepted")
	}
}

func TestBackendConfigRejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, json.RawMessage(`{"platform":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBackendConfig(path); err == nil {
		t.Fatal("malformed backend config accepted")
	}
	if _, err := config.BucketName("c360", "invalid platform stem"); err == nil {
		t.Fatal("test requires invalid platform stem")
	}
}
