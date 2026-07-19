package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCheckHTTPHealth(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{name: "healthy", statusCode: http.StatusOK},
		{name: "unhealthy", statusCode: http.StatusServiceUnavailable, wantErr: "got 503, want 200"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("method = %s, want GET", req.Method)
				}
				return &http.Response{
					StatusCode: tc.statusCode,
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			})

			err := checkHTTPHealth(context.Background(), client, "http://127.0.0.1:8080/health")
			if tc.wantErr == "" && err != nil {
				t.Fatalf("checkHTTPHealth() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("checkHTTPHealth() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}
