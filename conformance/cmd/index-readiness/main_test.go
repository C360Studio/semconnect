package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/nats-io/nats.go/jetstream"
)

type fakeStatusEntry struct {
	value     []byte
	operation jetstream.KeyValueOp
}

func (e fakeStatusEntry) Bucket() string                  { return "GRAPH_STATUS" }
func (e fakeStatusEntry) Key() string                     { return "graph-index" }
func (e fakeStatusEntry) Value() []byte                   { return e.value }
func (e fakeStatusEntry) Revision() uint64                { return 1 }
func (e fakeStatusEntry) Created() time.Time              { return time.Time{} }
func (e fakeStatusEntry) Delta() uint64                   { return 0 }
func (e fakeStatusEntry) Operation() jetstream.KeyValueOp { return e.operation }

type scriptedWatcher struct {
	updates chan jetstream.KeyValueEntry
	stopped bool
}

func (w *scriptedWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *scriptedWatcher) Stop() error {
	w.stopped = true
	return nil
}

type scriptedWatchSource struct {
	watcher *scriptedWatcher
	err     error
	calls   int
}

func (s *scriptedWatchSource) WatchIndexStatus(context.Context) (jetstream.KeyWatcher, error) {
	s.calls++
	return s.watcher, s.err
}

func sourceForStatuses(t *testing.T, statuses ...graph.IndexStatusResponse) *scriptedWatchSource {
	t.Helper()
	watcher := &scriptedWatcher{updates: make(chan jetstream.KeyValueEntry, len(statuses))}
	for _, status := range statuses {
		data, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		watcher.updates <- fakeStatusEntry{value: data, operation: jetstream.KeyValuePut}
	}
	close(watcher.updates)
	return &scriptedWatchSource{watcher: watcher}
}

func testWaitConfig() waitConfig {
	return waitConfig{
		StableSamples: 2,
		Now: func() time.Time {
			return time.Date(2026, time.July, 31, 1, 2, 3, 0, time.UTC)
		},
	}
}

func TestWaitForReadinessDoesNotAcceptReadyBelowCapturedTarget(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 10, IndexedRevision: 7},
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 10, IndexedRevision: 8},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 9},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 10},
	)
	var evidence bytes.Buffer

	result, err := waitForReadiness(context.Background(), source, testWaitConfig(), &evidence)
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if result.Attempts != 4 {
		t.Fatalf("update count = %d, want 4", result.Attempts)
	}
	if result.TargetRevision != 10 || result.IndexedRevision != 10 {
		t.Fatalf("result revisions = (%d, %d), want (10, 10)",
			result.TargetRevision, result.IndexedRevision)
	}
	if !source.watcher.stopped {
		t.Fatal("status watcher was not stopped")
	}
}

func TestWaitForReadinessReturnsCaughtUpStatus(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 12, IndexedRevision: 12},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 12, IndexedRevision: 12},
	)
	var evidence bytes.Buffer

	result, err := waitForReadiness(context.Background(), source, testWaitConfig(), &evidence)
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if result.TargetRevision != 12 || result.IndexedRevision != 12 {
		t.Fatalf("result revisions = (%d, %d), want (12, 12)",
			result.TargetRevision, result.IndexedRevision)
	}

	lines := strings.Split(strings.TrimSpace(evidence.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("evidence lines = %d, want 3; evidence=%q", len(lines), evidence.String())
	}
	var final evidenceEvent
	if err := json.Unmarshal([]byte(lines[2]), &final); err != nil {
		t.Fatalf("decode final evidence: %v", err)
	}
	if final.Phase != "final" || final.CapturedTargetRevision != 12 || final.FinalIndexedRevision != 12 {
		t.Fatalf("final evidence = %#v", final)
	}
	if final.Timestamp == "" {
		t.Fatal("final evidence timestamp is empty")
	}
}

func TestWaitForReadinessFailsImmediatelyOnResetRequired(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t, graph.IndexStatusResponse{
		State: graph.IndexStateResetRequired, Code: graph.ErrorCodeGraphStateResetRequired,
		Reason: "legacy predicate state",
	})

	_, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), graph.ErrorCodeGraphStateResetRequired) {
		t.Fatalf("error = %v, want %q", err, graph.ErrorCodeGraphStateResetRequired)
	}
}

func TestWaitForReadinessRejectsTargetRegressionAfterCapture(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 20, IndexedRevision: 18},
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 20, IndexedRevision: 19},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 19, IndexedRevision: 20},
	)
	var evidence bytes.Buffer

	_, err := waitForReadiness(context.Background(), source, testWaitConfig(), &evidence)
	if err == nil || !strings.Contains(err.Error(), "target revision regressed") {
		t.Fatalf("error = %v, want target regression failure", err)
	}
	if !strings.Contains(evidence.String(), `"phase":"target-regression"`) ||
		!strings.Contains(evidence.String(), `"captured_target_revision":20`) ||
		!strings.Contains(evidence.String(), `"final_observed_target_revision":19`) {
		t.Fatalf("regression evidence missing comparison: %s", evidence.String())
	}
}

func TestWaitForReadinessRequiresCoverageOfAdvancedCurrentTarget(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 10, IndexedRevision: 8},
		graph.IndexStatusResponse{State: graph.IndexStateBuilding, TargetRevision: 10, IndexedRevision: 9},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 12, IndexedRevision: 10},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 12, IndexedRevision: 12},
	)

	result, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if result.Attempts != 4 {
		t.Fatalf("update count = %d, want 4", result.Attempts)
	}
	if result.TargetRevision != 10 || result.IndexedRevision != 12 {
		t.Fatalf("result revisions = (%d, %d), want captured=10 indexed=12",
			result.TargetRevision, result.IndexedRevision)
	}
}

func TestWaitForReadinessRejectsCaughtUpPreBootstrapEnvelope(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady,
			TargetRevision: 10, IndexedRevision: 10},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady,
			TargetRevision: 10, IndexedRevision: 10},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 10},
	)

	result, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if result.Attempts != 3 {
		t.Fatalf("accepted pre-bootstrap envelope after %d updates, want 3", result.Attempts)
	}
}

func TestWaitForReadinessRejectsCaughtUpDegradedEnvelope(t *testing.T) {
	t.Parallel()

	source := sourceForStatuses(t,
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateDegraded, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 10},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateDegraded, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 10},
		graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady, BootstrapComplete: true,
			TargetRevision: 10, IndexedRevision: 10},
	)

	result, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if result.Attempts != 3 {
		t.Fatalf("accepted degraded envelope after %d updates, want 3", result.Attempts)
	}
}

func TestWaitForReadinessRejectsMalformedUpdate(t *testing.T) {
	t.Parallel()

	watcher := &scriptedWatcher{updates: make(chan jetstream.KeyValueEntry, 1)}
	watcher.updates <- fakeStatusEntry{value: []byte("not-json"), operation: jetstream.KeyValuePut}
	close(watcher.updates)
	source := &scriptedWatchSource{watcher: watcher}

	_, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "decode graph index readiness status") {
		t.Fatalf("error = %v, want clear decode error", err)
	}
}

func TestWaitForReadinessRejectsDeletedStatusKey(t *testing.T) {
	t.Parallel()

	watcher := &scriptedWatcher{updates: make(chan jetstream.KeyValueEntry, 1)}
	watcher.updates <- fakeStatusEntry{operation: jetstream.KeyValueDelete}
	close(watcher.updates)
	source := &scriptedWatchSource{watcher: watcher}

	_, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "was deleted") {
		t.Fatalf("error = %v, want deleted-key error", err)
	}
}

func TestWaitForReadinessSurfacesWatchOpenFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("bucket unavailable")
	source := &scriptedWatchSource{err: want}
	_, err := waitForReadiness(context.Background(), source, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "open graph index readiness watch") {
		t.Fatalf("error = %v, want clear watch error", err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want errors.Is(_, %v)", err, want)
	}
}

func TestWaitForReadinessHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	source := &scriptedWatchSource{watcher: &scriptedWatcher{
		updates: make(chan jetstream.KeyValueEntry),
	}}

	_, err := waitForReadiness(ctx, source, testWaitConfig(), &bytes.Buffer{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}
