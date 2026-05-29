# Upstream ask — semstreams: header-classified structured request/reply errors

**Repo:** <https://github.com/C360Studio/semstreams>
**Issue:** [C360Studio/semstreams#93](https://github.com/C360Studio/semstreams/issues/93)
**Status:** **OPEN as of semstreams `v1.0.0-beta.86`.**

## What semconnect still needs

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

`gateway/cs-api` still uses `classifyEntityQueryError` to parse the
framework's current `error: <message>` body convention on entity-query
failures. That parser exists only because the response body is the
classification surface today.

The workaround should be removed when #93 ships. At that point
semconnect should read the structured classification and stop matching
human-readable error text.

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
