package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGatewayStartupSignalAbortsBlockedStart(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	signalCtx, signal := context.WithCancel(context.Background())
	defer signal()
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- startGatewayWithAbort(runtimeCtx, signalCtx, cancelRuntime, func(ctx context.Context) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		}, func(context.Context) error { return nil })
	}()
	<-entered
	signal()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("startup abort lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown signal did not abort blocked gateway startup")
	}
}

func TestGatewaySignalAfterStartupLeavesRuntimeLiveForDrain(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	signalCtx, signal := context.WithCancel(context.Background())
	defer signal()
	if err := startGatewayWithAbort(runtimeCtx, signalCtx, cancelRuntime, func(context.Context) error { return nil }, func(context.Context) error { t.Fatal("successful startup cleaned up early"); return nil }); err != nil {
		t.Fatal(err)
	}
	signal()
	if runtimeCtx.Err() != nil {
		t.Fatal("post-start signal canceled authority before Stop could drain")
	}
}

func TestGatewayStartupSignalRaceJoinsAcquiredResources(t *testing.T) {
	runtimeCtx, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()
	signalCtx, signal := context.WithCancel(context.Background())
	defer signal()
	cleaned := false
	err := startGatewayWithAbort(runtimeCtx, signalCtx, cancelRuntime, func(context.Context) error {
		signal()
		return nil
	}, func(ctx context.Context) error {
		if _, bounded := ctx.Deadline(); !bounded || ctx.Err() != nil {
			t.Fatal("cleanup lost independent bounded authority")
		}
		cleaned = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || !cleaned {
		t.Fatalf("startup race cleanup: err=%v cleaned=%v", err, cleaned)
	}
}
