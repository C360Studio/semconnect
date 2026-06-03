# Telemetry Graph Demo Runbook

This guide is for sponsors and early adopters who want to run the SemConnect
telemetry graph demo without first learning the whole SemStreams framework.

The demo shows a SvelteKit graph UI backed by the CS API gateway and a
SemStreams graph index. A small water-plant telemetry story is seeded through
HTTP, indexed by SemStreams, and visualized in the browser.

## What The Demo Shows

- CS API resources are created through SemConnect: Systems, Datastreams,
  Observations, a ControlStream, and a Feasibility record.
- SemStreams stores and indexes the semantic graph behind those resources.
- The UI renders entities, relationships, telemetry context, and natural
  language graph focus for queries such as `latest water temperature telemetry`.
- The comparison runner can execute the same story against the statistical
  graph tier and the semantic tier so we can see what semantic enrichment adds.

This is a demo and tuning harness, not a production deployment recipe. The
conformance harness remains the source of truth for CS API standards coverage.

## Prerequisites

- macOS or Linux with Docker Desktop or Docker Engine plus Compose v2.
- Node.js and npm.
- A local `semstreams` checkout next to this repo, or a custom path supplied
  with `SEMSTREAMS_DIR` or `--semstreams-dir`.
- Ports available for the default run:
  - `5179` for the SvelteKit dev server.
  - `48080` for direct CS API access.
  - `48081` for the Caddy browser proxy.
  - SemStreams tier ports in the `38080` and `38180` ranges.

From the UI directory, install dependencies once:

```bash
cd ui
npm install
```

## Quick UI Fixture Mode

Use this when you only want to see the browser experience with local demo data.
It is fast, does not start SemStreams, and is useful for screenshots or UI
review.

```bash
cd ui
npm run dev -- --host 127.0.0.1 --port 5179
```

Then browse to:

```text
http://127.0.0.1:5179
```

The fixture mode is intentionally local. It proves the interaction model, but
it does not prove CS API writes, graph indexing, or semantic search.

## Full-Stack Comparison Mode

Use this when you want to show the real integration path.

```bash
cd ui
npm run compare:full-stack -- --profile statistical
```

To compare both tiers in one run:

```bash
npm run compare:full-stack -- --profile both
```

If `semstreams` is not checked out next to this repo:

```bash
npm run compare:full-stack -- --semstreams-dir /path/to/semstreams --profile both
```

The runner starts the SvelteKit dev server, writes temporary Caddy and Compose
configuration, starts the selected SemStreams tier, starts `cs-api-server`,
seeds demo resources through CS API, waits for concrete CS API and graph counts,
runs the UI through Playwright, captures screenshots, writes a JSON summary,
and tears the stack down.

The browser entry point for a kept stack is:

```text
http://127.0.0.1:48081
```

Caddy is part of the demo because it gives the browser one stable origin:

| Path | Target |
|---|---|
| `/` | SvelteKit dev server |
| `/cs-api/*` | SemConnect CS API server |
| `/graphql` | SemStreams graph gateway |
| `/semembed/*` | semembed, when the semantic profile is available |
| `/seminstruct/*` | seminstruct, when the semantic profile is available |

Useful runner flags:

| Flag | Purpose |
|---|---|
| `--profile statistical` | Run the fast statistical graph tier. |
| `--profile semantic` | Run the semantic tier. |
| `--profile both` | Run statistical first, then semantic. |
| `--keep-stack` | Leave the last stack running for manual browser inspection. |
| `--no-screenshots` | Skip Playwright screenshots. |
| `--ui-semantic-assist` | Route optional UI semembed/seminstruct assist in semantic mode. |
| `--ui-port`, `--cs-api-port`, `--proxy-port` | Move local ports if something is already bound. |

## Expected Results

The seeded CS API resources should stabilize at:

```text
systems=3 datastreams=2 observations=3 controlstreams=1 feasibility=1
```

The graph prefix query should find the seeded demo entities. In the current
demo data, that prefix count is expected to settle around `14`.

The statistical tier should feel tight and fast. A recent local run returned a
natural-language graph search count of `2` in a few milliseconds, and the UI
rendered `19` entities with `53` relationships.

The semantic tier should be broader and slower. A recent local run returned a
natural-language graph search count of `76` in about `15s`, and the UI rendered
`89` entities with `483` relationships. That extra recall is useful for showing
semantic possibility, but it is also the place where tuning matters.

For a crisp sponsor demo, start with the statistical tier. Use the semantic
tier when the audience is ready to discuss enrichment, recall, and ranking.

## ID Shapes And CS API Mapping

Two dotted-string shapes show up in this system. They are easy to confuse.

SemStreams entity IDs are six-part resource IDs. The gateway config uses
five-part prefixes, then appends a final token derived from the client-provided
identifier.

For example, the demo config uses prefixes like:

```text
c360.demo.water.plant.system
c360.demo.water.plant.datastream
```

Those become six-part entity IDs such as:

```text
c360.demo.water.plant.system.pump-alpha
c360.demo.water.plant.datastream.inlet-temperature
```

The comparison runner already writes the right demo prefixes. Operators only
need to care about the five-part prefix rule when they customize
`cs-api.config.json`; using four or six parts in a prefix will fail validation.

CS API clients do not need to construct every ID by hand, but they do need to
understand which field is the source identifier:

| CS API request | Source identifier | Stored graph identity |
|---|---|---|
| `POST /systems` GeoJSON Feature | `properties.uid` | Six-part System entity ID. |
| `POST /systems` SensorML | `uniqueId` | Six-part System entity ID. |
| `POST /datastreams` | `id`, when already six-part | Honored as the Datastream entity ID. |
| `POST /datastreams` | `id`, when not six-part | Minted from `datastream_id_prefix`. |
| `POST /controlstreams` | `id`, when supplied | ControlStream entity ID or mint source. |
| `POST /feasibility` | `id`, when supplied | Feasibility entity ID or mint source. |

Three-part dotted names are predicates, not resource IDs. Examples include
gateway-local terms such as `cs-api.feasibility.status` or
`cs-api.deployment.parent`. They describe a fact about a resource in the graph.
The CS API response maps those facts back into fields and links such as
`status`, `controlstream@id`, `parent@id`, and collection membership.

The short version: six-part strings identify resources; three-part strings
describe relationships or fields. Early adopters mostly interact with the CS API
resource shapes, and graph inspectors will see the lower-level predicates.

## Batch Timing

The runner polls concrete state because the demo crosses several async
boundaries:

- CS API writes publish graph mutations and observations.
- SemStreams indexes graph entities after those writes.
- Natural-language search can lag behind prefix search, especially in the
  semantic tier.
- The UI waits until the graph surface reports ready before Playwright captures
  the result.

Counts can appear in stages. A brief mismatch between CS API counts and graph
counts is normal during startup; a count that does not move for a full timeout
is not.

## Troubleshooting

If Docker fails early, confirm Docker is running and that the default ports are
available. Use the port flags when another local service is already bound.

If the first run is slow, Docker may be building images and pulling Caddy. The
second run is usually much faster.

If the semantic tier looks noisy, use that as tuning evidence rather than a
demo failure. The statistical tier is currently the better default for a concise
telemetry walkthrough.

If GraphQL search reports `empty entity_id`, that is a known SemStreams graph
gateway routing issue triggered by nested `relationships` fields inside a
`globalSearch` query. The UI avoids that shape now, and the upstream issue is
tracked at:

```text
https://github.com/C360Studio/semstreams/issues/206
```

If you run with `--keep-stack`, the runner prints the temporary directory that
contains the generated Compose override. To tear that stack down manually, run
the printed Compose command from the `semstreams` checkout with `down -v`.

## Verification Commands

For UI code and browser behavior:

```bash
cd ui
npm run check
npm run test:e2e
```

For CS API standards conformance:

```bash
./conformance/run.sh
```

The conformance harness is separate from the demo runner. It validates the
gateway against the pinned OGC CS API ETS; the demo runner validates that the
story people see in the browser is backed by real CS API and SemStreams paths.
