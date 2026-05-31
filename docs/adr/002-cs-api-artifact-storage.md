# ADR-S002 - CS API graph and artifact storage pattern

- **Status**: Accepted (2026-05-31)
- **Repo**: `semconnect`
- **Upstream**: [semstreams #171](https://github.com/C360Studio/semstreams/issues/171) closed in
  `v1.0.0-beta.90`

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

- a dotted internal predicate from a System, Procedure, Datastream, or related resource to a source document
  artifact, registered to the CS API `hasSource` IRI for RDF/JSON-LD export
- a dotted internal predicate from a Datastream to a SWE schema artifact, registered to the CS API
  `hasResultSchema` IRI for RDF/JSON-LD export
- a dotted internal predicate from a ControlStream to a SWE command schema artifact, registered to the CS API
  `hasCommandSchema` IRI for RDF/JSON-LD export
- `csapi:SensorMLDocument` for SensorML source artifact type IRIs
- `csapi:SWESchemaDocument` for SWE Common schema artifact type IRIs

Semstreams `v1.0.0-beta.90` landed the CS API artifact classes and IRI constants. Its core vocabulary contract
still says graph predicates are three-level dotted names (`domain.category.property`) and IRIs are boundary
mappings, not internal predicate keys. [semstreams #182](https://github.com/C360Studio/semstreams/issues/182)
tracks dotted CS API predicate constants for direct graph use. Existing semconnect gateway-local schema
predicates remain temporary bridges until the gateway migrates Datastream and ControlStream schemas to typed
artifact entities with dotted relationship predicates.

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
2. Read the artifact relationship triple, such as `cs-api.datastream.resultSchema`.
3. Fetch the artifact entity from graph.
4. Use the artifact entity's `StorageRef` to fetch content from ObjectStore.
5. Decode the content with the domain parser, such as `pkg/swecommon` or SensorML.

## Migration Implications

Current semconnect schema storage remains valid as a temporary bridge:

- `cs-api.datastream.schema` stores Datastream result schema JSON locally.
- ControlStream command schema storage follows the same gateway-local pattern.

Next local migration: create `csapi:SWESchemaDocument` artifact entities, relate Datastreams with a dotted
result-schema relationship predicate, relate ControlStreams with a dotted command-schema relationship predicate,
register those predicates to the CS API IRIs for export, and retire the local JSON predicates.
