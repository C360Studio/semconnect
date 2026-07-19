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
	"github.com/nats-io/nats.go"
)

type scriptedReply struct {
	status graph.IndexStatusResponse
	raw    []byte
	err    error
}

type scriptedRequester struct {
	replies []scriptedReply
	calls   int
}

type contextRequester struct{}

func (contextRequester) RequestIndexStatus(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *scriptedRequester) RequestIndexStatus(context.Context) ([]byte, error) {
	if r.calls >= len(r.replies) {
		return nil, errors.New("script exhausted")
	}
	reply := r.replies[r.calls]
	r.calls++
	if reply.raw != nil || reply.err != nil {
		return reply.raw, reply.err
	}
	data, err := json.Marshal(reply.status)
	if err != nil {
		panic(err)
	}
	return data, nil
}

func testWaitConfig() waitConfig {
	return waitConfig{
		StableSamples: 2,
		PollInterval:  0,
		Now: func() time.Time {
			return time.Date(2026, time.July, 18, 1, 2, 3, 0, time.UTC)
		},
	}
}

func TestWaitForReadinessDoesNotAcceptReadyBelowCapturedTarget(t *testing.T) {
	t.Parallel()

	requester := &scriptedRequester{replies: []scriptedReply{
		{status: graph.IndexStatusResponse{TargetRevision: 10, IndexedRevision: 7}},
		{status: graph.IndexStatusResponse{TargetRevision: 10, IndexedRevision: 8}},
		{status: graph.IndexStatusResponse{Ready: true, TargetRevision: 10, IndexedRevision: 9}},
		{status: graph.IndexStatusResponse{Ready: true, TargetRevision: 10, IndexedRevision: 10}},
	}}
	var evidence bytes.Buffer

	result, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &evidence)
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if requester.calls != 4 {
		t.Fatalf("request count = %d, want 4", requester.calls)
	}
	if result.TargetRevision != 10 || result.IndexedRevision != 10 {
		t.Fatalf("result revisions = (%d, %d), want (10, 10)",
			result.TargetRevision, result.IndexedRevision)
	}
}

func TestWaitForReadinessReturnsCaughtUpStatus(t *testing.T) {
	t.Parallel()

	requester := &scriptedRequester{replies: []scriptedReply{
		{status: graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady,
			TargetRevision: 12, IndexedRevision: 12}},
		{status: graph.IndexStatusResponse{Ready: true, State: graph.IndexStateReady,
			TargetRevision: 12, IndexedRevision: 12}},
	}}
	var evidence bytes.Buffer

	result, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &evidence)
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

	requester := &scriptedRequester{replies: []scriptedReply{{status: graph.IndexStatusResponse{
		State:  graph.IndexStateResetRequired,
		Code:   graph.ErrorCodeGraphStateResetRequired,
		Reason: "legacy predicate state",
	}}}}

	_, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), graph.ErrorCodeGraphStateResetRequired) {
		t.Fatalf("error = %v, want %q", err, graph.ErrorCodeGraphStateResetRequired)
	}
	if requester.calls != 1 {
		t.Fatalf("request count = %d, want immediate failure after 1", requester.calls)
	}
}

func TestWaitForReadinessRejectsTargetRegressionAfterCapture(t *testing.T) {
	t.Parallel()

	requester := &scriptedRequester{replies: []scriptedReply{
		{status: graph.IndexStatusResponse{TargetRevision: 20, IndexedRevision: 18}},
		{status: graph.IndexStatusResponse{TargetRevision: 20, IndexedRevision: 19}},
		{status: graph.IndexStatusResponse{Ready: true, TargetRevision: 19, IndexedRevision: 20}},
	}}
	var evidence bytes.Buffer

	_, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &evidence)
	if err == nil || !strings.Contains(err.Error(), "target revision regressed") {
		t.Fatalf("error = %v, want target regression failure", err)
	}
	if requester.calls != 3 {
		t.Fatalf("request count = %d, want immediate failure after 3", requester.calls)
	}
	if !strings.Contains(evidence.String(), `"phase":"target-regression"`) ||
		!strings.Contains(evidence.String(), `"captured_target_revision":20`) ||
		!strings.Contains(evidence.String(), `"final_observed_target_revision":19`) {
		t.Fatalf("regression evidence missing comparison: %s", evidence.String())
	}
}

func TestWaitForReadinessRequiresCoverageOfAdvancedCurrentTarget(t *testing.T) {
	t.Parallel()

	requester := &scriptedRequester{replies: []scriptedReply{
		{status: graph.IndexStatusResponse{TargetRevision: 10, IndexedRevision: 8}},
		{status: graph.IndexStatusResponse{TargetRevision: 10, IndexedRevision: 9}},
		{status: graph.IndexStatusResponse{Ready: true, TargetRevision: 12, IndexedRevision: 10}},
		{status: graph.IndexStatusResponse{Ready: true, TargetRevision: 12, IndexedRevision: 12}},
	}}

	result, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("waitForReadiness() error = %v", err)
	}
	if requester.calls != 4 {
		t.Fatalf("request count = %d, want 4", requester.calls)
	}
	if result.TargetRevision != 10 || result.IndexedRevision != 12 {
		t.Fatalf("result revisions = (%d, %d), want captured=10 indexed=12",
			result.TargetRevision, result.IndexedRevision)
	}
}

func TestWaitForReadinessRejectsMalformedResponse(t *testing.T) {
	t.Parallel()

	requester := &scriptedRequester{replies: []scriptedReply{{raw: []byte("not-json")}}}

	_, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "decode graph index status") {
		t.Fatalf("error = %v, want clear decode error", err)
	}
}

func TestWaitForReadinessSurfacesTimeoutAndNoResponder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "timeout", err: nats.ErrTimeout},
		{name: "no responder", err: nats.ErrNoResponders},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requester := &scriptedRequester{replies: []scriptedReply{{err: tt.err}}}

			_, err := waitForReadiness(context.Background(), requester, testWaitConfig(), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "request graph index status") {
				t.Fatalf("error = %v, want clear request error", err)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.err)
			}
		})
	}
}

func TestWaitForReadinessHonorsContextDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()

	_, err := waitForReadiness(ctx, contextRequester{}, testWaitConfig(), &bytes.Buffer{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
}
