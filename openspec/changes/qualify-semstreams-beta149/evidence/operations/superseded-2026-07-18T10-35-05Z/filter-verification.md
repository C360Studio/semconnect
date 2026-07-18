# Shutdown filter verification

The operator used this case-insensitive failure expression against the bounded
beta.149 shutdown log:

```text
heartbeat service not running
metrics forwarder not running
Service stop failed
Error stopping services
graceful shutdown failed
"level":"ERROR"
```

The alternatives above were joined with `|` for the actual `rg -i` command.

The expression was first sanity-checked against the archived beta.147 failure
shape. It matched both archived rehearsals, including `Service stop failed`,
`Error stopping services`, `graceful shutdown failed`, and
`heartbeat service not running`. The exact historical matches remain in:

```text
conformance/output/semstreams-backend-restart-2026-07-18T02-03-23Z.log
conformance/output/semstreams-backend-restart-archived-2026-07-18T02-03-23Z.log
```

Applying the same expression to `shutdown-window.log` returned zero matches.
Every line in that bounded beta.149 window is INFO. The final line is
`SemStreams shutdown complete`. Container metadata independently records exit
code 0, `OOMKilled=false`, an empty runtime error, SIGTERM signal 15, and no
SIGKILL event. Silence is therefore not the sole basis for the pass.

Decision: pass. None of the issue #549 or generic teardown failure signatures
occurred, and the process exited cleanly inside the 30-second grace period.
