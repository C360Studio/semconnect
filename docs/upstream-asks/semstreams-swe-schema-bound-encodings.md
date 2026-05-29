# Upstream ask — semstreams: schema-bound SWE Common encodings

**Repo:** <https://github.com/C360Studio/semstreams>
**Issue:** [C360Studio/semstreams#116](https://github.com/C360Studio/semstreams/issues/116)
**Status:** **OPEN as of semstreams `v1.0.0-beta.87`.**

## What semconnect still needs

semconnect needs framework-level SWE Common encoders/decoders that bind
values to the described schema for:

- SWE JSON
- SWE text/CSV
- SWE binary
- observation read-back
- command payload/schema parity

## Current semconnect behavior

Stage 27 exposes observation-value subsets for:

- `application/swe+json`
- `application/swe+csv`
- `application/swe+binary`

Those responses intentionally carry `X-CS-SWE-Subset:
observation-values`. semconnect does not claim SWE Common conformance,
and command-side SWE payload execution remains out of scope at v0.1.

## Why it matters

The gateway can format simple values, but it should not own the
schema-bound SWE model. That belongs in semstreams with the rest of the
SWE/SOSA/OMS primitives so observations and commands share one encoding
contract.
