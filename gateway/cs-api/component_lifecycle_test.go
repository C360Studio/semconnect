package csapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func startLifecycleGateway(t *testing.T) (*Component, context.Context, context.CancelFunc) {
	t.Helper()
	nats := startEmbeddedNATSServer(t, true)
	cfg := DefaultConfig()
	cfg.StandaloneServer, cfg.BindAddress = true, "127.0.0.1:0"
	c, err := New(cfg, connectSemStreamsClient(t, nats.ClientURL()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Initialize(); err != nil {
		t.Fatal(err)
	}
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	if err := c.Start(runtimeCtx); err != nil {
		cancelRuntime()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		_ = c.Stop(stopCtx)
		cancelRuntime()
	})
	return c, runtimeCtx, cancelRuntime
}

func TestGatewayStopDrainsUnderLiveRuntimeAuthority(t *testing.T) {
	c, runtimeCtx, _ := startLifecycleGateway(t)
	entered, release, stopped := make(chan context.Context, 1), make(chan struct{}), make(chan struct{})
	c.httpMux.HandleFunc("GET /lifecycle-reference", func(w http.ResponseWriter, r *http.Request) {
		entered <- r.Context()
		<-release
		_, _ = w.Write([]byte("completed"))
	})
	c.httpServer.RegisterOnShutdown(func() { close(stopped) })
	requestDone := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + c.httpListener.Addr().String() + "/lifecycle-reference")
		if err == nil {
			defer resp.Body.Close()
			var body []byte
			body, err = io.ReadAll(resp.Body)
			if string(body) != "completed" {
				err = errors.New("admitted response did not complete")
			}
		}
		requestDone <- err
	}()
	requestCtx := <-entered
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- c.Stop(stopCtx) }()
	<-stopped
	if runtimeCtx.Err() != nil || requestCtx.Err() != nil {
		t.Error("graceful stop canceled admitted runtime authority")
	}
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("completed repeated stop: %v", err)
	}
}

func TestGatewayRuntimeCancellationReachesAdmittedRequests(t *testing.T) {
	c, _, cancelRuntime := startLifecycleGateway(t)
	entered, requestCanceled := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c.httpMux.HandleFunc("GET /lifecycle-reference", func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
			close(requestCanceled)
		case <-release:
		}
	})
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		resp, err := http.Get("http://" + c.httpListener.Addr().String() + "/lifecycle-reference")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	cancelRuntime()
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Error("Start runtime cancellation did not reach admitted request")
	}
}

func TestGatewayStopRejectsNilAndUsesCanceledCallerContext(t *testing.T) {
	c, _, _ := startLifecycleGateway(t)
	if err := c.Stop(nil); err == nil {
		t.Fatal("nil Stop context accepted")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	c.httpMux.HandleFunc("GET /lifecycle-reference", func(_ http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
	})
	go func() {
		resp, err := http.Get("http://" + c.httpListener.Addr().String() + "/lifecycle-reference")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop did not retain caller cancellation: %v", err)
	}
}

func TestGatewayStartRequiresLiveAuthorityAndInitialization(t *testing.T) {
	c := newTestComponent(t, nil)
	if err := c.Start(nil); err == nil {
		t.Fatal("nil Start authority accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Start authority accepted: %v", err)
	}
	if err := c.Start(context.Background()); err == nil {
		t.Fatal("Start without Initialize accepted")
	}
}

func TestGatewayStartReportsOccupiedListener(t *testing.T) {
	c, runtimeCtx, _ := startLifecycleGateway(t)
	if err := c.Start(runtimeCtx); err != nil {
		t.Fatalf("already running Start: %v", err)
	}
	cfg := c.cfg
	cfg.BindAddress = c.httpListener.Addr().String()
	other, err := New(cfg, c.nats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := other.Start(runtimeCtx); err == nil {
		t.Fatal("occupied listener silently accepted")
	}
	if err := other.Stop(runtimeCtx); err != nil {
		t.Fatalf("failed Start cleanup: %v", err)
	}
}
