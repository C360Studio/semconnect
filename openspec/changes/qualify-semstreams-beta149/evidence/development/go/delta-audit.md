# Beta.148 and beta.149 consumer audit

The complete upstream source delta was reviewed from beta.147 commit
`5cc22c109594e48b7f1cec04bcaaf0106d85495a` through beta.149 commit
`7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`.

Besides PR #550's service lifecycle changes, the delta touches agentic state and execution policy,
`processor/agentic-loop`, `processor/agentic-tools`, `processor/rule`, rule schemas, and their tests/docs. Semconnect
does not configure or directly invoke those changed runtime behaviors. Some shared agentic packages are nevertheless
compiled transitively and are explicitly qualified below.

## Source audit

Command searched production and test Go source for imports of changed agentic-loop, agentic-tools, or rule packages:

```console
$ rg -n 'github\.com/c360studio/semstreams/(agentic|processor/(agentic-loop|agentic-tools|rule))' \
    --glob '*.go' cmd conformance gateway message parser pkg vocabulary
```

Result: zero direct imports. The guarded audit exited `0` and reported:

```text
PASS: no source imports of changed agentic-loop, agentic-tools, or rule packages
```

This does not mean that no agentic package is compiled. The production dependency inventory contains:

```text
github.com/c360studio/semstreams/agentic
github.com/c360studio/semstreams/agentic/agentrun
```

Their exact dependency paths are:

```text
semconnect/gateway/cs-api -> semstreams/component -> semstreams/agentic
semconnect/gateway/cs-api -> semstreams/gateway -> semstreams/service -> semstreams/agentic/agentrun
```

Those shared packages supply framework component/service types. Semconnect does not directly call the changed
iteration-budget, execution-policy, advertised-tool, spawn, or recovery behavior. Their transitive compilation is
covered by the complete unit, race, integration, vet, and build matrix; it is not represented as absence from the
binary.

`go mod why` also reports processor package paths through SemStreams dependency tests:

```text
semstreams/service.test -> semstreams/componentregistry -> semstreams/processor/agentic-loop
semstreams/service.test -> semstreams/componentregistry -> semstreams/processor/agentic-tools
semstreams/service.test -> semstreams/processor/rule
```

The processor packages are absent from `go list -deps -test ./...` for semconnect itself because Go does not compile
dependency-module tests into semconnect's tests. The focused upstream service suite separately compiles the PR #550
service tests and their test graph.

## Executable configuration and subject audit

Command searched executable configuration and source for `agentic-loop`, `agentic-tools`, `publish_agent` /
`publish-agent`, and rule action/execute forms. Result: zero matches. The guarded audit exited `0` and reported:

```text
PASS: no executable config or source references changed agentic/rule behaviors
```

The authoritative configured service, component, and subject inventory is:

```json
{"services":["component-manager","service-manager"],"components":["graph-index","graph-index-spatial","graph-index-temporal","graph-ingest","graph-query"],"subjects":["ALIAS_INDEX","ENTITY_STATES","ENTITY_STATES","ENTITY_STATES","ENTITY_STATES","INCOMING_INDEX","OUTGOING_INDEX","PREDICATE_INDEX","SPATIAL_INDEX","TEMPORAL_INDEX","_semconnect.unused.ingest","graph.query.>"]}
```

The configuration's top-level `tier: "rules"` is platform placement metadata. There is no rule processor component,
rule-action configuration, or rule-action subject. Every enabled component is one of the five graph processors above.

The resolved module graph contains one SemStreams version:

```text
github.com/c360studio/semstreams v1.0.0-beta.149
```

Disposition: the changed agentic-loop, agentic-tools, publish-agent, and rule-action runtime behaviors are not
configured or invoked by semconnect. Shared agentic packages are transitively compiled and pass every required gate;
processor packages occur only in the upstream dependency-test graph. No runtime consumer finding requires a separate
behavior specification.
