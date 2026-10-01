package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestShutdownRetainsRuntimeAuthorityAndExactStopContext(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	stopCtx, cancelStop := context.WithTimeout(context.Background(), time.Second)
	defer cancelStop()
	var order []string
	wantErr := errors.New("service cleanup")
	resources := backendResources{
		stopServices: func(ctx context.Context) error {
			if ctx != stopCtx || runtimeCtx.Err() != nil {
				t.Fatal("stop lost exact context or running authority")
			}
			order = append(order, "services")
			return wantErr
		},
		stopConfig: func(timeout time.Duration) error {
			if timeout <= 0 || timeout > time.Second || runtimeCtx.Err() != nil {
				t.Fatal("invalid config cleanup authority")
			}
			order = append(order, "config")
			return nil
		},
		closeNATS: func(ctx context.Context) error {
			if ctx != stopCtx || runtimeCtx.Err() != nil {
				t.Fatal("NATS close lost authority")
			}
			order = append(order, "nats")
			return nil
		},
	}
	if err := resources.stop(stopCtx); !errors.Is(err, wantErr) {
		t.Fatalf("lost cleanup failure: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"services", "config", "nats"}) {
		t.Fatalf("cleanup order %v", order)
	}
}

func TestShutdownRejectsNilBeforeAction(t *testing.T) {
	resources := backendResources{closeNATS: func(context.Context) error { t.Fatal("nil context acted"); return nil }}
	if err := resources.stop(nil); err == nil {
		t.Fatal("nil stop context accepted")
	}
}

func TestIdentityProvisioningRejectsMismatchWithoutOverwrite(t *testing.T) {
	expected := platformIdentity{Org: "c360", Stem: "semconnect", ID: "semconnect"}
	for _, test := range []struct {
		name, body string
		ok         bool
	}{
		{"matching", `{"id":"semconnect","stem":"semconnect","org":"c360"}`, true},
		{"other authority", `{"id":"semconnect-ab1234","stem":"semconnect","org":"c360"}`, false},
		{"corrupt", `{"id":`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := verifyIdentity([]byte(test.body), expected)
			if (err == nil) != test.ok {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestBackendRegistryContainsOnlyRetainedComponents(t *testing.T) {
	registry, err := backendRegistry()
	if err != nil {
		t.Fatal(err)
	}
	factories := registry.ListFactories()
	want := map[string]bool{"graph-ingest": true, "graph-index": true, "graph-index-spatial": true, "graph-index-temporal": true, "graph-query": true, "objectstore": true}
	if len(factories) != len(want) {
		t.Fatalf("registered %v", factories)
	}
	for _, factory := range factories {
		if !want[factory.Name] {
			t.Errorf("unexpected component %s", factory.Name)
		}
	}
}

func TestStartupCancellationAbortsAndJoinsSignalCallback(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(t.Context())
	defer cancelRuntime()
	bootCtx, cancelBoot := context.WithCancel(t.Context())
	defer cancelBoot()
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- startWithAbort(runtimeCtx, bootCtx, cancelRuntime, func(runtime, boot context.Context) error {
			close(entered)
			<-runtime.Done()
			return runtime.Err()
		})
	}()
	<-entered
	cancelBoot()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup cancellation did not reach owner")
	}
}

func TestSignalAfterStartupKeepsRuntimeLiveForControlledStop(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(t.Context())
	defer cancelRuntime()
	bootCtx, cancelBoot := context.WithCancel(t.Context())
	defer cancelBoot()
	if err := startWithAbort(runtimeCtx, bootCtx, cancelRuntime, func(context.Context, context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	cancelBoot()
	if runtimeCtx.Err() != nil {
		t.Fatal("post-start signal prematurely cancelled runtime")
	}
}
