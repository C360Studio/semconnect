# Production deployment static audit

## Scope and provenance

The local checkout and GitHub `main` tree were searched for deployment,
infrastructure, release, rollback, reseed, image-registry, and production NATS
metadata. Disposable conformance output and all beta.147/beta.149 signed
evidence were excluded as production sources.

The only semconnect GitHub workflow is `.github/workflows/conformance.yml`
(SHA-256 `ed10ace930fa0de3917b15d919e13b4b8e7816b425cbdf235c61e1718c7c4490`).
It grants `contents: read`, runs `./conformance/run.sh`, and uploads local
conformance artifacts. It has no GitHub environment, deployment step, registry
login, image publication, or production target.

The remote `main` tree at
`a8d1a7d15a94e3a5ede3ead27c668458fec2ede1` contains no production deployment
descriptor. Files whose paths contain `deployment` are OGC API domain sources
and gateway code, not infrastructure.

## Defaults are not rendered production values

`gateway/cs-api/config.go` (SHA-256
`02b75f83c7a31ffd677a6a22e3f280ccb02cf23b03e3b1651506640620a13779`)
explicitly says its defaults satisfy development and production overrides them
through JSON configuration. The development defaults include:

- observation stream `CS_API_OBSERVATIONS`;
- observation subject prefix `cs-api.observations`;
- schema ObjectStore `CS_API_ARTIFACTS`;
- one replica and unlimited byte caps for both retained resource families;
- the `c360.semconnect.systems.csapi.*` ID prefixes.

`cmd/cs-api-server/main.go` (SHA-256
`9acbf0634404a4d0b8bef3606aa259a14669cdbf8ebd7c015032794019f77e3f`)
defaults NATS to `nats://localhost:4222` and describes NATS URL as a deployment
concern. None of these defaults establishes a literal production setting.

`ENTITY_STATES` and `SPATIAL_INDEX` occur in the disposable conformance
SemStreams configuration. They may be framework defaults, but no rendered
production config proves their actual names, account, domain, counts, or
revisions. They are therefore unresolved in the production manifest.

## Writer inventory

Static code establishes only potential writer classes:

- cs-api mutation handlers request `graph.mutation.entity.create_with_triples`,
  `update_with_triples`, and `delete`;
- cs-api observation POST publishes under the configured observation subject;
- cs-api startup ensures the observation stream and schema ObjectStore;
- SemStreams graph-ingest owns entity-state writes and graph indexes.

Static code cannot prove which binaries, replicas, external producers,
administrative tools, or scheduled jobs are active in production. A runtime
owner-supplied writer inventory is still mandatory before any stop or wipe.

## Deployment, reseed, and rollback

The repository has a build-only `Dockerfile` but no production registry path,
published digest, release, GitHub deployment, or committed deployment revision.
It contains no authoritative production reseed URI/revision/checksum/count,
literal reseed command, maintenance window, rollback owner, or prior image
digests. Conformance fixtures and conformance image IDs are explicitly rejected
for these fields.

The inherited ADR-S003, schema, and template remain authoritative requirements:

- ADR SHA-256: `3e0b20d68a16c1e33c4528c0054a8d4359ca1236ee8d855d1eb892c5d94f6147`;
- schema SHA-256: `d9fa3c931ee60a5b4f1cd0b361e0d1c2ec1bb3af31f344e7ae0d17b6ec91a312`;
- template SHA-256: `8d6480b8cb5805b82fdf4a9bca846f47e2b01df8dca41d0861fb8dc92dfcf88f`.

No production approval is warranted from this discovery.
