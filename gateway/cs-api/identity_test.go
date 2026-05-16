package csapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityMiddleware_AlwaysPopulatesContext(t *testing.T) {
	tests := []struct {
		name          string
		fwdUser       string
		fwdEmail      string
		wantUser      string
		wantEmail     string
		wantVerified  bool
		wantSubject   string
		wantAuditHdr  string // a header key that must appear in AuditHeaders()
		dontWantAudit string // a header key that must NOT appear
	}{
		{
			name:          "no proxy headers → anonymous, no audit headers leak",
			wantVerified:  false,
			wantSubject:   "",
			dontWantAudit: "X-CS-Subject",
		},
		{
			name:         "reverse-proxy user is recorded for audit, not for authz",
			fwdUser:      "alice@example.org",
			wantUser:     "alice@example.org",
			wantVerified: false,
			wantAuditHdr: "X-CS-Forwarded-User",
		},
		{
			name:         "reverse-proxy email is recorded for audit",
			fwdEmail:     "bob@example.org",
			wantEmail:    "bob@example.org",
			wantAuditHdr: "X-CS-Forwarded-Email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Identity
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = IdentityFrom(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/systems", nil)
			if tt.fwdUser != "" {
				req.Header.Set("X-Forwarded-User", tt.fwdUser)
			}
			if tt.fwdEmail != "" {
				req.Header.Set("X-Forwarded-Email", tt.fwdEmail)
			}
			IdentityMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

			if got.Verified != tt.wantVerified {
				t.Errorf("Verified: got %v want %v", got.Verified, tt.wantVerified)
			}
			if got.Subject != tt.wantSubject {
				t.Errorf("Subject: got %q want %q", got.Subject, tt.wantSubject)
			}
			if tt.wantUser != "" && got.Forwarded["User"] != tt.wantUser {
				t.Errorf("Forwarded[User]: got %q want %q", got.Forwarded["User"], tt.wantUser)
			}
			if tt.wantEmail != "" && got.Forwarded["Email"] != tt.wantEmail {
				t.Errorf("Forwarded[Email]: got %q want %q", got.Forwarded["Email"], tt.wantEmail)
			}

			audit := got.AuditHeaders()
			if tt.wantAuditHdr != "" {
				if _, ok := audit[tt.wantAuditHdr]; !ok {
					t.Errorf("AuditHeaders missing %q (got %v)", tt.wantAuditHdr, audit)
				}
			}
			if tt.dontWantAudit != "" {
				if _, ok := audit[tt.dontWantAudit]; ok {
					t.Errorf("AuditHeaders should not contain %q (got %v)", tt.dontWantAudit, audit)
				}
			}
		})
	}
}

func TestIdentityFrom_DefaultsToAnonymous(t *testing.T) {
	// Handlers must never see a nil Identity, even if middleware was bypassed.
	id := IdentityFrom(context.Background())
	if id.Verified {
		t.Errorf("default identity Verified=true; want false")
	}
	if id.Forwarded == nil {
		t.Errorf("default identity Forwarded map is nil; handlers expect non-nil")
	}
}
