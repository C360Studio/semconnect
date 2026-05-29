# RESOLVED upstream ask — semstreams: schema-bound SWE Common encodings

**Repo:** <https://github.com/C360Studio/semstreams>
**Issue:** [C360Studio/semstreams#116](https://github.com/C360Studio/semstreams/issues/116)
**Status:** **RESOLVED in `v1.0.0-beta.88`** via semstreams PR #166 /
ADR-050. The new `pkg/swecommon` package ships schema-bound
`DataRecord` values plus JSON, text/CSV, and binary encoders/decoders.

## What semconnect needed

semconnect needs framework-level SWE Common encoders/decoders that bind
values to the described schema for:

- SWE JSON
- SWE text/CSV
- SWE binary
- observation read-back
- command payload/schema parity

## Current semconnect behavior

Stage 32 exposes observation-value subsets for:

- `application/swe+json`
- `application/swe+csv`
- `application/swe+binary`

Those responses now route through semstreams `pkg/swecommon` using a
v0.1 inferred `{time,result}` `DataRecord`. They still intentionally
carry `X-CS-SWE-Subset: observation-values` because Datastream resources
do not yet advertise result schemas. semconnect does not claim SWE
Common conformance, and command-side SWE payload execution remains out
of scope at v0.1.

That is now local semconnect follow-up work, not an upstream framework
blocker. The next gateway step is to bind datastream result schemas and
controlstream command schemas to `pkg/swecommon.DataRecord`, then drop
the subset header.

## Why it matters

The gateway can format simple values, but it should not own the
schema-bound SWE model. That belongs in semstreams with the rest of the
SWE/SOSA/OMS primitives so observations and commands share one encoding
contract.

## Resolution notes

semstreams beta.88 provides the shared model and encoder/decoder
surface. No additional semstreams ask blocks CS API SWE work in
semconnect.
