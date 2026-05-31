# ADR-S002 - CS API graph and artifact storage pattern

- **Status**: Proposed (2026-05-30)
- **Repo**: `semconnect`
- **Upstream**: [semstreams #171](https://github.com/C360Studio/semstreams/issues/171)

## Context

CS API resources mix two kinds of data:

- graphable facts: identity, type, relationships, names, time ranges, query filters, and other small scalar values
- artifacts: original source documents, SWE Common schemas, command schemas, media, binary content, and other
  structured blobs

Earlier semconnect notes described Datastream and ControlStream schema storage as waiting for a future
StorageRef-backed primitive. semstreams #171 clarified that no broad framework primitive change is needed.
The substrate already supports ObjectStore, `StorageReference`, `ContentStorable`, singular
`EntityState.StorageRef`, and entity mutations that preserve `StorageRef`.

The missing piece for CS API is a convention: when does a payload belong in graph, and when does it become a
stored artifact related to graph state?

## Decision

Use graph for facts that should be queryable, joinable, and visible as semantic relationships:

- entity identity and class
- parent/child and resource relationships
- references between CS API resources
- small scalar values used by filters, collection responses, and discovery
- artifact relationships, such as "this Datastream has result schema X"

Use ObjectStore for content whose internal shape should stay owned by a domain parser or encoder:

- original SensorML documents
- SWE Common `DataRecord` schemas
- ControlStream command schemas
- large structured JSON documents
- binary content and media
- one-off source payloads needed for lossless read-back

For reusable, versioned, or independently discoverable CS API artifacts, use first-class artifact entities:

1. Create an artifact entity with its own 6-part EntityID.
2. Store the artifact content in ObjectStore.
3. Put the artifact `StorageReference` on the artifact entity's singular `EntityState.StorageRef`.
4. Relate the parent CS API entity to the artifact entity with a vocabulary predicate.

This keeps graph state semantic and queryable while keeping object-shaped content out of triples.

## CS API Artifact Roles

The target semstreams vocabulary shape from #171 is:

- `csapi:HasSource` from a System, Procedure, Datastream, or related resource to a source document artifact
- `csapi:HasResultSchema` from a Datastream to a SWE schema artifact
- `csapi:HasCommandSchema` from a ControlStream to a SWE command schema artifact
- `csapi:SensorMLDocument` for SensorML source artifacts
- `csapi:SWESchemaDocument` for SWE Common schema artifacts

Until those vocabulary constants land in semstreams, semconnect may keep narrow gateway-local predicates as
bridges. Those bridges should be treated as migration points, not permanent vocabulary forks.

## Pattern Selection

Use first-class artifact entities when the artifact is reusable, versioned, independently discoverable, or has
its own lifecycle. CS API `source`, `resultSchema`, and `commandSchema` all fit this pattern.

Use a bundled `BinaryStorable` or `ContentStorable` under the parent entity's singular `StorageRef` only when
the content is constitutive of that one entity and will not be reused or independently queried. Examples include
a multimedia document entity whose video and thumbnail are just fields of the document.

Do not add a `map[string]*StorageReference` to semconnect-side entity state. It would make typed artifact roles
opaque to graph queries, duplicate reusable schemas, and create two competing ways to express stored content.

## Read Path

The gateway read path for artifact-backed fields is:

1. Fetch the parent entity from graph.
2. Read the artifact relationship triple, such as `csapi:HasResultSchema`.
3. Fetch the artifact entity from graph.
4. Use the artifact entity's `StorageRef` to fetch content from ObjectStore.
5. Decode the content with the domain parser, such as `pkg/swecommon` or SensorML.

## Migration Implications

Current semconnect schema storage remains valid as a temporary bridge:

- `cs-api.datastream.schema` stores Datastream result schema JSON locally.
- ControlStream command schema storage follows the same gateway-local pattern.

Once semstreams ships the CS API artifact vocabulary constants, migrate those fields to typed artifact entities
and retire the local JSON predicates.
