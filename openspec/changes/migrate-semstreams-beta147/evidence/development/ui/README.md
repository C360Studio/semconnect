# Svelte development evidence

This evidence covers OpenSpec tasks 4.1 through 4.5 for `migrate-semstreams-beta147`. Task 4.6 remains a joint
Go/Svelte handoff gate and is intentionally open until the Go development lane is ready.

## Failing-first contract

The semantic-label contract was added before its implementation:

```text
$ npm --prefix ui run test:e2e -- --grep "owns stable CS API labels"
Error: Cannot find module 'ui/src/lib/semantics/semanticCatalog'
Error: No tests found
exit 1
```

The red result proves the product-owned label catalog was absent. The initial implementation then received two
blocking reviewer findings. Both remediations were also executed failing-first:

```text
$ npm --prefix ui run test:e2e -- --grep "complete frozen semantic ledger"
SyntaxError: semanticCatalog does not provide an export named MIGRATED_SEMANTIC_FIELDS
exit 1

$ npm --prefix ui run test:e2e -- --grep "controlled properties as scalar metadata"
SyntaxError: c360.demo... is not valid JSON
exit 1
```

The first remediation test loads the frozen architecture ledger and requires an exact 32-entry catalog: missing,
extra, or stale entries fail. The second requires serialized ControlledProperty metadata and rejects a relationship
with that predicate. Both targeted cases passed after implementation, and the complete suite passed `7 passed`.

## Implementation scope

- Added one product-owned semantic catalog with explicit labels and descriptions for all 32 frozen semantic-ledger
  predicates. The ledger-wide test proves those identities never use fallback storage-name humanization.
- Migrated demo, live CS API adapter, graph-gateway fixtures, relationship colors, and Playwright fixtures from
  historical `rdf.type`, camelCase, and pre-ledger relationship identities to the frozen canonical identities.
- Changed the entity detail surface to render explicit product labels instead of raw storage predicates.
- Corrected the ControlStream demo so `controlled-properties` carries the same serialized metadata shape as the Go
  producer and is not modeled or colored as an entity relationship.
- Preserved standards-shaped CS API JSON member keys such as `phenomenonTime`, `resultTime`, and `commandFormat` at
  the HTTP/UI boundary; they are display-mapped and are not rewritten as graph-storage fields.
- Kept existing TypeScript public interfaces intact. An audit found no generated OpenAPI client or generated
  TypeScript catalog in `ui`, and no removed SemStreams Go package path appears in UI source.

## Six demo prefix classifications

Command:

```sh
node ui/scripts/full-stack-compare.mjs --validate-identities
```

The validator resolves the template before classifying it, requires five canonical prefix parts, applies the exact
segment grammar, and reserves one separator plus the 66-byte `h-` and full SHA-256 instance token.

| Configuration | Resolved prefix | Prefix bytes | Digest-form entity bytes | Classification |
|---|---|---:|---:|---|
| `system_id_prefix` | `c360.demo.water.plant.system` | 28 | 95 | valid five-part prefix |
| `datastream_id_prefix` | `c360.demo.water.plant.datastream` | 32 | 99 | valid five-part prefix |
| `controlstream_id_prefix` | `c360.demo.water.plant.controlstream` | 35 | 102 | valid five-part prefix |
| `command_id_prefix` | `c360.demo.water.plant.command` | 29 | 96 | valid five-part prefix |
| `feasibility_id_prefix` | `c360.demo.water.plant.feasibility` | 33 | 100 | valid five-part prefix |
| `schema_artifact_id_prefix` | `c360.demo.water.plant.schema` | 28 | 95 | valid five-part prefix |

All six are valid runtime five-part prefixes. The unresolved template spellings are not graph entity IDs and are
therefore classified by resolved value rather than falsely treated as invalid six-part entities.

## Fresh command evidence

Environment: Node `v22.20.0`, npm `10.9.3`, Playwright `1.60.0`.

```text
$ npm --prefix ui run check
svelte-check found 0 errors and 0 warnings
exit 0

$ npm --prefix ui run build
client and server production builds completed
exit 0

$ npm --prefix ui run test:e2e
7 passed (3.7s)
exit 0

$ git diff --check -- ui
no output
exit 0
```

`ui/package.json` exposes no separate unit-test command. The pure semantic-catalog contract is run as a browserless
Playwright test in the required `test:e2e` suite; browser-backed label, accessibility, demo, live-adapter, semantic
search, and telemetry cases remain in the same seven-test gate.

The historical-identity audit returned no production UI matches for the migrated `rdf.type`, `isHostedBy`,
`producedBy`, `observedProperty`, `controlsSystem`, command/control camelCase, or Feasibility relationship spellings.
The sole old spelling retained is a negative Playwright assertion proving it is not rendered.

The scalar-metadata audit finds `controlled-properties` only as a ControlStream fact. No relationship label, demo
relationship, or edge-color entry exists for that predicate.
