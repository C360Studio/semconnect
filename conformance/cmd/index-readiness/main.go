// Command index-readiness captures a stable post-seed graph target revision
// and blocks until graph-index has authoritatively indexed that revision.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/readiness"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

type statusWatchSource interface {
	WatchIndexStatus(context.Context) (jetstream.KeyWatcher, error)
}

type natsStatusWatchSource struct {
	client *natsclient.Client
}

func (s natsStatusWatchSource) WatchIndexStatus(ctx context.Context) (jetstream.KeyWatcher, error) {
	bucket, err := s.client.GetKeyValueBucket(ctx, readiness.BucketGraphStatus)
	if err != nil {
		return nil, fmt.Errorf("open readiness bucket %s: %w", readiness.BucketGraphStatus, err)
	}
	watcher, err := bucket.Watch(ctx, readiness.KeyGraphIndex, jetstream.UpdatesOnly())
	if err != nil {
		return nil, fmt.Errorf("watch readiness key %s/%s: %w",
			readiness.BucketGraphStatus, readiness.KeyGraphIndex, err)
	}
	return watcher, nil
}

type waitConfig struct {
	StableSamples int
	Now           func() time.Time
}

type readinessResult struct {
	TargetRevision  uint64
	IndexedRevision uint64
	Attempts        int
}

type evidenceEvent struct {
	Timestamp               string `json:"timestamp"`
	Phase                   string `json:"phase"`
	Attempt                 int    `json:"attempt,omitempty"`
	Ready                   bool   `json:"ready,omitempty"`
	State                   string `json:"state,omitempty"`
	Code                    string `json:"code,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	IndexedRevision         uint64 `json:"indexed_revision,omitempty"`
	TargetRevision          uint64 `json:"target_revision,omitempty"`
	Lag                     uint64 `json:"lag,omitempty"`
	CapturedTargetRevision  uint64 `json:"captured_target_revision,omitempty"`
	FinalIndexedRevision    uint64 `json:"final_indexed_revision,omitempty"`
	FinalObservedTarget     uint64 `json:"final_observed_target_revision,omitempty"`
	StableTargetSampleCount int    `json:"stable_target_sample_count,omitempty"`
	Error                   string `json:"error,omitempty"`
}

func waitForReadiness(
	ctx context.Context,
	source statusWatchSource,
	cfg waitConfig,
	evidence io.Writer,
) (readinessResult, error) {
	if cfg.StableSamples < 2 {
		return readinessResult{}, errors.New("stable sample count must be at least 2")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	encoder := json.NewEncoder(evidence)
	watcher, err := source.WatchIndexStatus(ctx)
	if err != nil {
		watchErr := fmt.Errorf("open graph index readiness watch: %w", err)
		_ = writeEvidence(encoder, evidenceEvent{
			Timestamp: nowUTC(cfg.Now), Phase: "watch", Error: watchErr.Error(),
		})
		return readinessResult{}, watchErr
	}
	defer watcher.Stop()

	phase := "capture"
	var lastTarget uint64
	stableSamples := 0
	var capturedTarget uint64
	attempt := 0
	for {
		var entry jetstream.KeyValueEntry
		select {
		case <-ctx.Done():
			waitErr := fmt.Errorf("wait for graph index readiness: %w", ctx.Err())
			_ = writeEvidence(encoder, evidenceEvent{
				Timestamp: nowUTC(cfg.Now), Phase: phase, Attempt: attempt, Error: waitErr.Error(),
			})
			return readinessResult{}, waitErr
		case update, ok := <-watcher.Updates():
			if !ok {
				watchErr := errors.New("graph index readiness watch closed")
				_ = writeEvidence(encoder, evidenceEvent{
					Timestamp: nowUTC(cfg.Now), Phase: phase, Attempt: attempt, Error: watchErr.Error(),
				})
				return readinessResult{}, watchErr
			}
			if update == nil {
				continue
			}
			entry = update
		}

		attempt++
		if graph.IsKVTombstone(entry.Operation()) {
			tombstoneErr := fmt.Errorf("graph index readiness key %s/%s was deleted",
				readiness.BucketGraphStatus, readiness.KeyGraphIndex)
			_ = writeEvidence(encoder, evidenceEvent{
				Timestamp: nowUTC(cfg.Now), Phase: phase, Attempt: attempt, Error: tombstoneErr.Error(),
			})
			return readinessResult{}, tombstoneErr
		}

		var status graph.IndexStatusResponse
		if err := json.Unmarshal(entry.Value(), &status); err != nil {
			decodeErr := fmt.Errorf("decode graph index readiness status: %w", err)
			_ = writeEvidence(encoder, evidenceEvent{
				Timestamp: nowUTC(cfg.Now), Phase: phase, Attempt: attempt, Error: decodeErr.Error(),
			})
			return readinessResult{}, decodeErr
		}

		if status.TargetRevision > 0 && status.TargetRevision == lastTarget {
			stableSamples++
		} else if status.TargetRevision > 0 {
			lastTarget = status.TargetRevision
			stableSamples = 1
		} else {
			lastTarget = 0
			stableSamples = 0
		}
		if err := writeEvidence(encoder, eventForStatus(cfg.Now, phase, attempt, stableSamples, status)); err != nil {
			return readinessResult{}, fmt.Errorf("archive graph index readiness status: %w", err)
		}

		if status.Code == graph.ErrorCodeGraphStateResetRequired || status.State == graph.IndexStateResetRequired {
			return readinessResult{}, fmt.Errorf("%s: %s", graph.ErrorCodeGraphStateResetRequired, status.Reason)
		}
		if capturedTarget == 0 && stableSamples >= cfg.StableSamples {
			capturedTarget = status.TargetRevision
			phase = "watch"
		}
		if capturedTarget > 0 && status.TargetRevision < capturedTarget {
			regressionErr := fmt.Errorf(
				"graph index target revision regressed: current=%d captured=%d",
				status.TargetRevision,
				capturedTarget,
			)
			_ = writeEvidence(encoder, evidenceEvent{
				Timestamp:              nowUTC(cfg.Now),
				Phase:                  "target-regression",
				Attempt:                attempt,
				CapturedTargetRevision: capturedTarget,
				FinalIndexedRevision:   status.IndexedRevision,
				FinalObservedTarget:    status.TargetRevision,
				Error:                  regressionErr.Error(),
			})
			return readinessResult{}, regressionErr
		}
		proceed, _ := graph.EvaluateReadinessGate(graph.StatusReading{Status: status, Fresh: true})
		if capturedTarget > 0 && status.BootstrapComplete && proceed && status.Ready &&
			status.TargetRevision >= capturedTarget &&
			status.IndexedRevision >= capturedTarget && status.IndexedRevision >= status.TargetRevision {
			result := readinessResult{
				TargetRevision: capturedTarget, IndexedRevision: status.IndexedRevision, Attempts: attempt,
			}
			if err := writeEvidence(encoder, evidenceEvent{
				Timestamp:              nowUTC(cfg.Now),
				Phase:                  "final",
				Attempt:                attempt,
				Ready:                  status.Ready,
				State:                  status.State,
				CapturedTargetRevision: capturedTarget,
				FinalIndexedRevision:   status.IndexedRevision,
				FinalObservedTarget:    status.TargetRevision,
			}); err != nil {
				return readinessResult{}, fmt.Errorf("archive final graph index readiness status: %w", err)
			}
			return result, nil
		}
	}
}

func eventForStatus(
	now func() time.Time,
	phase string,
	attempt int,
	stableSamples int,
	status graph.IndexStatusResponse,
) evidenceEvent {
	return evidenceEvent{
		Timestamp:               nowUTC(now),
		Phase:                   phase,
		Attempt:                 attempt,
		Ready:                   status.Ready,
		State:                   status.State,
		Code:                    status.Code,
		Reason:                  status.Reason,
		IndexedRevision:         status.IndexedRevision,
		TargetRevision:          status.TargetRevision,
		Lag:                     status.Lag,
		StableTargetSampleCount: stableSamples,
	}
}

func nowUTC(now func() time.Time) string {
	return now().UTC().Format(time.RFC3339Nano)
}

func writeEvidence(encoder *json.Encoder, event evidenceEvent) error {
	return encoder.Encode(event)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "index-readiness: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("index-readiness", flag.ContinueOnError)
	natsURL := flags.String("nats-url", "nats://127.0.0.1:4222", "NATS server URL")
	output := flags.String("output", "", "JSON Lines evidence output path (required)")
	timeout := flags.Duration("timeout", 60*time.Second, "overall readiness timeout")
	stableSamples := flags.Int("stable-samples", 2, "consecutive equal non-zero target updates")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("-output is required")
	}
	if *timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	evidence, err := os.OpenFile(*output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open evidence output: %w", err)
	}
	defer evidence.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client, err := natsclient.NewClient(*natsURL)
	if err != nil {
		return fmt.Errorf("create NATS client: %w", err)
	}
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to NATS at %s: %w", *natsURL, err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = client.Close(closeCtx)
	}()

	result, err := waitForReadiness(ctx, natsStatusWatchSource{client: client}, waitConfig{
		StableSamples: *stableSamples,
		Now:           time.Now,
	}, evidence)
	if err != nil {
		return err
	}
	fmt.Printf("graph index ready: target_revision=%d indexed_revision=%d updates=%d evidence=%s\n",
		result.TargetRevision, result.IndexedRevision, result.Attempts, *output)
	return nil
}
