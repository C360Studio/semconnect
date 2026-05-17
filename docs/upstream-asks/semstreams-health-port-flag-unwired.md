# Upstream ask — semstreams: `-health-port` flag declared but never wired

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 9 conformance backend integration (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** **Filed at [C360Studio/semstreams#100](https://github.com/C360Studio/semstreams/issues/100) (OPEN, as of 2026-05-16; still OPEN at v1.0.0-beta.75 — `cfg.HealthPort` still parsed and never read).** semconnect workaround: `conformance/compose.yml` healthcheck targets the service-manager's `:8090/health` (the actually-bound endpoint) instead of the flag's nominal port.

## Summary

`cmd/semstreams/flags.go` declares, parses, and validates the
`-health-port` flag (defaults to 8080, env `SEMSTREAMS_HEALTH_PORT`),
but `cfg.HealthPort` is **never read by any code path in the
repository**. The flag is dead code, and the help text — *"Health
check port, 0 to disable"* — is misleading: setting `-health-port 8090`
neither binds `/health` on 8090 nor disables it when set to 0.

`/health` is actually served by the `service-manager` service on its
configured `http_port` (a separate field under `services.service-manager.config`).
This is non-obvious from the flag's existence and help text, and we
spent ~30 minutes diagnosing a "healthcheck not responding" issue in
a Docker Compose harness because of it.

## File / line refs

- `cmd/semstreams/flags.go:19` — `HealthPort int` field on `Config`.
- `cmd/semstreams/flags.go:57` — `flag.IntVar(&cfg.HealthPort, "health-port", 8080, ...)`.
- `cmd/semstreams/flags.go:106-107` — `Validate()` range-checks the value.
- **No other reference to `cfg.HealthPort` or `HealthPort` in `cmd/semstreams/`** (grep confirms).

`grep -rn "HealthPort\b" /path/to/semstreams/cmd/semstreams/` returns
only the three declarations/parses/validates above. Likewise for
`-health-port` flag-name string searches in `cmd/`.

## Proposed change

Pick one of the following — both are valid, the user-impact difference
is moot since the flag is dead today:

### Option A — wire the flag

In `cmd/semstreams/main.go` (or wherever `service.Manager` is wired),
add a parallel listener that binds `cfg.HealthPort` and serves the
existing `/health` handler from the service-manager when `cfg.HealthPort
!= 0`. This matches the flag's documented intent (a dedicated
health-port listener independent of the human-facing service-manager
UI).

### Option B — remove the flag

Drop the `HealthPort` field, the `flag.IntVar` call, and the range
check from `flags.go`. Update operator-facing docs to point at
`services.service-manager.config.http_port` as the single source of
`/health`. Cleaner from a "less to maintain" standpoint, but breaks
any operator who's been passing `-health-port` (silent no-op today,
but their muscle memory will fight the change).

Either way, the help text should match reality.

## Why this is worth filing

A flag that does nothing is worse than a missing flag — it actively
misleads operators about the framework's port layout. This came up
when wiring `semstreams-backend` into the `semconnect` conformance
Docker Compose harness; the healthcheck pointed at `localhost:8080/health`
(the default `-health-port`) and got connection-refused for the
entire 900s healthcheck window. Replicated locally with a standalone
container before figuring out that the actual `/health` was on the
service-manager's port.

If Option B is chosen, also clean up the env var (`SEMSTREAMS_HEALTH_PORT`)
and the `docker/Dockerfile` `HEALTHCHECK` line if it references it
(check needed).
