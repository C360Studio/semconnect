package csapi

import (
	"context"
	"net/http"
)

// Identity is the authenticated principal for an HTTP request.
//
// At v0.1 every request carries Anonymous(). The middleware boundary is in
// place so a future ADR can drop in JWT / mTLS verification without touching
// handler code: handlers always read Identity from the context, and audit
// headers always flow through to NATS publishes regardless of how the
// identity was established.
type Identity struct {
	// Subject is the verified principal identifier (sub claim, mTLS CN, …)
	// when authentication is on, or empty when anonymous.
	Subject string

	// Forwarded carries trust-on-faith hints from an upstream reverse proxy
	// (X-Forwarded-User, X-Forwarded-Email). At v0.1 these are recorded for
	// audit only — handlers must not make authorization decisions on them.
	Forwarded map[string]string

	// Verified is true only when the server itself validated the identity.
	// At v0.1 this is always false.
	Verified bool
}

// Anonymous returns the v0.1 default identity attached to every request.
func Anonymous() Identity {
	return Identity{Forwarded: map[string]string{}}
}

// AuditHeaders returns the headers a NATS publish should carry so downstream
// consumers can record who triggered the publish. Always returns a non-nil
// map; callers can safely merge into it.
//
// First consumer lands at Stage 3 (POST /datastreams/{id}/observations) when
// the publish call needs to ship these onto the BaseMessage envelope. The
// seam lives here at Stage 2 so the audit trail starts honest from the first
// mutation, no handler edit required.
func (id Identity) AuditHeaders() map[string]string {
	h := make(map[string]string, len(id.Forwarded)+2)
	if id.Subject != "" {
		h["X-CS-Subject"] = id.Subject
	}
	if id.Verified {
		h["X-CS-Verified"] = "true"
	}
	for k, v := range id.Forwarded {
		h["X-CS-Forwarded-"+k] = v
	}
	return h
}

type identityKey struct{}

// IdentityFrom returns the Identity attached to ctx. The zero-value Anonymous
// identity is returned if no middleware has run — but in production every
// request flows through WithIdentity first, so handlers can rely on it.
func IdentityFrom(ctx context.Context) Identity {
	if id, ok := ctx.Value(identityKey{}).(Identity); ok {
		return id
	}
	return Anonymous()
}

// withIdentity attaches id to ctx for downstream handlers.
func withIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityMiddleware wraps next so every request carries an Identity in its
// context. v0.1 records X-Forwarded-* headers from a trusted reverse proxy
// but does not verify anything. Replacing the body of this function with a
// JWT / mTLS verifier is the entire mechanical change to add real auth —
// handlers stay unchanged.
func IdentityMiddleware(next http.Handler) http.Handler {
	const (
		fwdUserHeader  = "X-Forwarded-User"
		fwdEmailHeader = "X-Forwarded-Email"
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := Anonymous()
		if u := r.Header.Get(fwdUserHeader); u != "" {
			id.Forwarded["User"] = u
		}
		if e := r.Header.Get(fwdEmailHeader); e != "" {
			id.Forwarded["Email"] = e
		}
		next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
	})
}
