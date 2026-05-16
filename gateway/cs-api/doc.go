// Package csapi implements the OGC API Connected Systems v1.0 HTTP gateway
// over the semstreams framework primitives.
//
// The component implements:
//   - [component.Discoverable]      — registry-side metadata + health
//   - [component.LifecycleComponent] — Initialize / Start / Stop
//   - [gateway.Gateway]              — RegisterHTTPHandlers(prefix, mux)
//
// Scope at v0.1 is fixed by ADR-S001 (docs/adr/001-cs-api-server-scope.md):
// Core + JSON + GeoJSON + SensorML + OMS + JSON-LD conformance classes,
// Accept-header content negotiation, anonymous-but-auditable identity
// (real auth lands behind the same middleware seam), single binary.
package csapi
