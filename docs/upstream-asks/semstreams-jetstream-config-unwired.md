# Upstream ask — semstreams: `nats.jetstream` config block declared but never applied

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 9 conformance backend integration (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** **Filed at [C360Studio/semstreams#101](https://github.com/C360Studio/semstreams/issues/101) (OPEN, as of v1.0.0-beta.79 — `MaxFileStore` declared but not applied to the connected server).** semconnect workaround: `conformance/nats.conf` pins JetStream limits at the nats-server level (10GB file / 1GB memory) via `jetstream { max_file_store: 10GB; max_memory_store: 1GB }`.

## Summary

`config/config.go:130` declares a `NATSConfig.JetStream` field of type
`JetStreamConfig`:

```go
type JetStreamConfig struct {
    Enabled           bool   `json:"enabled"`
    Domain            string `json:"domain,omitempty"`
    MaxMemory         int64  `json:"max_memory,omitempty"`
    MaxFileStore      int64  `json:"max_file_store,omitempty"`
    RetentionPolicy   string `json:"retention_policy,omitempty"`
    ReplicationFactor int    `json:"replication_factor,omitempty"`
}
```

The struct is parsed from operator config (`nats.jetstream` block)
and stored on `Config.NATS.JetStream`. **No code in the repository
reads any of these fields** — `grep -rn "MaxFileStore\b"
/path/to/semstreams --include="*.go"` returns only the declaration
itself. Same for `MaxMemory`, `Domain`, `RetentionPolicy`,
`ReplicationFactor`.

In practice this means operators who want to size JetStream (e.g.
to absorb the framework's baseline streams — `LOGS` 100MB, `HEALTH`
10MB mem, `METRICS`, `FLOWS`) must configure nats-server **directly**
(via `-c nats.conf` with a `jetstream { max_file_store: 10GB }` block)
because the framework can't push those limits to the server it
connects to. The schema implies otherwise.

## Concrete impact

`nats-server 2.10`'s CLI doesn't expose JetStream sizing flags
(`-js -sd /data -m 8222` is the maximum; `max_file_store` is config-file
only). Auto-sizing on Docker for Mac under image-cache pressure can
compute a `max_file_store` too small for the framework's baseline
streams, surfacing as:

```
nats: API error: code=500 err_code=10047 description=insufficient
storage resources available
```

at stream-create time during framework boot. The natural operator
reaction is to set `nats.jetstream.max_file_store: 10GB` in the
framework config — and that does nothing. Diagnosing this took
~20 minutes of "the schema accepts it, the value is parsed, the
streams still fail to create" confusion.

## File / line refs

- `config/config.go:130` — `JetStream JetStreamConfig` on `NATSConfig`.
- `config/config.go:141-149` — full `JetStreamConfig` struct definition.
- `config/config.go:411` — defaults applied to the struct.
- `config/config.go:863` — `c.NATS.JetStream = aux.NATS.JetStream` in custom unmarshal.
- **No other reference to `MaxFileStore` / `MaxMemory` / etc. in the codebase** beyond struct declaration / defaults / unmarshal.

## Proposed change

Pick one:

### Option A — wire the config (preferred)

In `config/streams.go` (or wherever the framework first creates
streams), use `natsClient`'s system API to set JetStream account
limits before creating streams:

```go
// pseudocode
if cfg.NATS.JetStream.MaxFileStore > 0 {
    js, _ := natsClient.JetStream()
    err := js.UpdateAccountInfo(&nats.AccountInfo{
        Limits: nats.AccountLimits{
            MaxFileStore: cfg.NATS.JetStream.MaxFileStore,
            MaxMemory:    cfg.NATS.JetStream.MaxMemory,
        },
    })
    // ...
}
```

Note: this only works for accounts the framework has permission to
update (typically dev / single-tenant). For multi-tenant setups,
this should be a server-side `nats.conf` concern, and the framework
should leave it alone.

### Option B — remove the config block

Drop the `JetStream` field from `NATSConfig` and document in
operator docs that JetStream sizing is a nats-server concern, not
a framework concern. Update the framework's example `configs/*.json`
to remove `nats.jetstream` blocks. Cleaner from a "framework
boundary" standpoint.

Either way, the schema and the behavior should match.

## Workaround in place (semconnect Stage 9)

`conformance/nats.conf`:

```
jetstream {
    store_dir: "/data"
    max_file_store: 10GB
    max_memory_store: 1GB
}
```

Mounted into the `nats` compose service with `-c /etc/nats/nats.conf`.
Works, but the lever lives outside the framework's config — which is
the wart.
