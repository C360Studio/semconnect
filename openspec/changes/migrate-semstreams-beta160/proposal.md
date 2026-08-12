## Why

SemStreams `v1.0.0-beta.160` removes the raw graph mutation, ownership, open port-definition, and permissive storage
contracts that semconnect currently uses. Exact entity reads also add an authoritative KV revision wrapper. A simple
module bump would fail compilation in several paths and could silently misread exact entity state in another.

Beta.160 requires every adopter to begin on newly provisioned NATS storage. Semconnect needs one coordinated,
greenfield migration that preserves the CS API surface while making concurrency, projection, artifact storage, audit,
configuration, and operational behavior explicit.

## What Changes

- **BREAKING** Replace raw graph mutation subjects with the typed `semstreams.graph.mutation/v1` family.
- **BREAKING** Make reconcile and delete exact-revision operations with no hidden retry; map revision conflicts to
  HTTP 409 and uncertain commits to an explicit HTTP 503 response.
- Decode exact reads through `graph.ExactEntity` and honor batch `missing[]` responses.
- Replace ownership bindings with resource-specific local projection contracts and valid message types.
- Restrict SensorML mutations to the primary root entity and drop foreign-subject inverse child facts.
- Make SWE schema artifacts immutable and content-addressed through storage instance `objectstore` and bucket
  `CS_API_ARTIFACTS`; defer orphan garbage collection.
- Use structured gateway mutation audit without requiring custom identity headers on the typed mutation wire.
- Regenerate component declarations in beta.160's canonical closed port model and align NATS to 2.14.
- Start beta.160 only on newly provisioned NATS storage and prove same-version stop/restart persistence.
- Preserve the claimed CS API surface and re-run unchanged external `137/0/0` conformance evidence.

## Non-goals

- Migrating or opening beta.159 NATS state, adding compatibility readers, or supporting mixed framework revisions.
- Retrying revision conflicts or uncertain commits automatically.
- Creating embedded SensorML children in a multi-entity transaction.
- Garbage-collecting orphan schema artifacts.
- Adopting unused beta.160 trajectory, tool, agent-run, metric, GraphQL, aggregate-client, or persistence features.

## Capabilities

### New Capabilities

- `semstreams-beta160-migration`: Exact release and NATS alignment, typed revision-fenced graph operations, local
  projection contracts, root-only SensorML projection, exact artifact storage, structured audit, canonical ports,
  fresh-state adoption, and unchanged external conformance.

### Modified Capabilities

None. The prior beta.159 qualification is historical and does not define beta.160's typed graph contracts.

## Impact

- Gateway graph state adapters, mutation helpers, exact and batch reads, error classification, and tests change.
- Projection contracts replace ownership bindings across all CS API resource families.
- SensorML graph output, schema artifact creation, audit evidence, OpenAPI errors, and configuration change.
- `go.mod`, `go.sum`, backend pins, NATS server pins, Compose files, and generated declarations move together.
- Deployment and conformance require newly provisioned NATS state; rollback remains a separate beta.159 environment.
- Release remains blocked until full Go and real-NATS gates, clean-volume persistence, external `137/0/0`, and
  independent no-weakening review are green.
