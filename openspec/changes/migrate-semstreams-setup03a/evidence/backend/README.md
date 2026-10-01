# Consumer graph host evidence

`cmd/cs-graph-backend` uses the exact shared SemStreams pin and registers only graph-ingest, graph-index,
graph-index-spatial, graph-index-temporal, graph-query, and ObjectStore component factories. It uses the builtin
payload registry plus SemConnect's eleven registered resource types and their configured contracts.

The initial unit-test compile failures are in `contract-red.log`; the subsequent signal/startup contract failures
are in `startup-red.log`. Final verification:

```bash
go test -race -tags=integration -count=1 ./cmd/cs-graph-backend
go vet -tags=integration ./cmd/cs-graph-backend
```

Both passed. The live test uses embedded NATS 2.14.4, file storage in a new `t.TempDir`, and server readiness;
no Docker or production server is reused. It proves explicit identity provisioning, conflict refusal without
rewrite, custom same-authority System prefix registration, actual HTTP create/read against the composed graph,
health readiness, controlled stop while runtime authority remains live, and absence of graph responders afterward.
A second live case forces an occupied HTTP listener and proves failed-start cleanup preserves the unrelated listener.
Signal tests synchronize on a startup gate: cancellation reaches the unfinished owner and the callback is joined;
after startup, signal cancellation leaves runtime authority live for controlled stop.

## Operational surface

- `-config PATH` loads framework backend configuration and starts the retained graph services.
- `-cs-api-config PATH` loads the same gateway JSON file for all consumer payload projection prefixes and contracts.
- `-validate` runs the upstream composition validator without connecting to NATS.
- `-provision-identity ID` performs an explicit one-shot atomic create of the documented platform identity record,
  verifies a matching existing record without writing, refuses a mismatch, and exits without starting graph services.

Ordinary startup never pre-creates an identity. It adopts or mints through the upstream configuration manager, then
uses the effective authority. Qualification explicitly provisions `c360.semconnect` before boot to preserve existing
public fixture identifiers; this is ADR-104's documented operational opt-out, not a hidden authority bypass.

The frozen upstream configuration manager still exposes `Stop(time.Duration)`. The host passes only the remaining
caller shutdown budget to that API; service StopAll and NATS Close receive the exact caller context. No replacement
cleanup deadline or detached cleanup owner is introduced. The process has one separately bounded 15-second stop.

This host does not solve the frozen target's prohibition on typed writes to foreign authorities. The paired
consumer HTTP reproduction and its migration blocker are separate evidence under `evidence/federation-gap/`.
