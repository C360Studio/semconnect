# RESOLVED upstream ask — semstreams: header-classified structured request/reply errors

**Repo:** <https://github.com/C360Studio/semstreams>
**Issue:** [C360Studio/semstreams#93](https://github.com/C360Studio/semstreams/issues/93)
**Status:** **RESOLVED for semconnect in `v1.0.0-beta.87`** via
semstreams PR #165. #93 remains open upstream for deferred Phase 4
breaking cleanup and follow-up hardening, but the additive
header-classified error path semconnect needs has shipped.

## What semconnect needed

NATS request/reply error responses should carry machine-readable
classification so gateways can map framework failures to HTTP status
without parsing prose. The desired shape is stable error headers and/or
a classified JSON body that distinguishes at least:

- entity not found
- invalid request / validation failure
- conflict / revision mismatch
- transient backend unavailable
- internal framework failure

## Current semconnect workaround

Stage 30 on beta.87 switches `gateway/cs-api` entity reads to
`natsclient.ClassifyReply`, which consumes `X-Status` /
`X-Error-Class` when present and keeps legacy body-prefix fallback
during the upstream dual-encoding window.

semconnect still maps `"not found: ..."` inside the Invalid class to
its local `errEntityNotFound` sentinel because HTTP `404` vs `400` is a
gateway-level distinction; semstreams intentionally does not add a
NotFound class in this phase.

## Why it matters

CS API endpoints need precise status mapping:

- unknown resource: `404`
- bad client input: `400`
- duplicate create / revision conflict: `409`
- backend unavailable: `503`
- unexpected framework failure: `500`

String parsing makes that contract fragile and forces semconnect to
track framework wording changes. Header-classified errors would let both
repos evolve without coupling on text.

## Resolution notes

No further semstreams action is required before semconnect can continue.
Open upstream follow-ups from #93 Phase 4 do not block the CS API read
path.
