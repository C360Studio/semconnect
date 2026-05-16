package csapi

import (
	"crypto/tls"
	"net/http"
	"testing"
)

// TestAbsoluteBase_Precedence pins the three-tier resolution order
// for absolute URLs in landing-page hrefs. The precedence is
// load-bearing — clients (the Botts ETS, pygeoapi, ldproxy)
// follow service-desc / data links via direct `URI.create + get`,
// not RFC 3986 base-resolution, so a wrong absolute URL takes the
// gateway out of conformance.
func TestAbsoluteBase_Precedence(t *testing.T) {
	cases := []struct {
		name        string
		host        string
		tls         bool
		fwdProto    string
		fwdHost     string
		wantBaseURL string
	}{
		{
			name:        "plain http, no proxy headers",
			host:        "cs-api-server:8080",
			wantBaseURL: "http://cs-api-server:8080",
		},
		{
			name:        "direct TLS termination on the gateway",
			host:        "cs.example.com",
			tls:         true,
			wantBaseURL: "https://cs.example.com",
		},
		{
			name:        "behind a TLS-terminating reverse proxy",
			host:        "cs-api-server:8080",
			fwdProto:    "https",
			fwdHost:     "api.example.com",
			wantBaseURL: "https://api.example.com",
		},
		{
			name:        "X-Forwarded-Proto wins over r.TLS (proxy is the source of truth)",
			host:        "cs.example.com",
			tls:         true,
			fwdProto:    "http",
			wantBaseURL: "http://cs.example.com",
		},
		{
			name:        "X-Forwarded-Host without proto falls back to scheme detection",
			host:        "cs-api-server:8080",
			fwdHost:     "api.example.com",
			wantBaseURL: "http://api.example.com",
		},
		{
			name:        "garbage X-Forwarded-Proto falls through to detection",
			host:        "cs.example.com",
			tls:         true,
			fwdProto:    "javascript:alert(1)",
			wantBaseURL: "https://cs.example.com",
		},
		{
			name:        "empty X-Forwarded-Proto with plain HTTP",
			host:        "cs-api-server:8080",
			fwdProto:    "",
			wantBaseURL: "http://cs-api-server:8080",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "/", nil)
			req.Host = tc.host
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tc.fwdProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.fwdProto)
			}
			if tc.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tc.fwdHost)
			}
			if got := absoluteBase(req); got != tc.wantBaseURL {
				t.Errorf("absoluteBase: got %q want %q", got, tc.wantBaseURL)
			}
		})
	}
}
