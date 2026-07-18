# Shutdown filter verification

The qualifying `10:52:31Z` rehearsal used these case-insensitive failure
alternatives against the bounded beta.149 shutdown log:

```text
heartbeat service not running
metrics forwarder not running
Service stop failed
Error stopping services
graceful shutdown failed
"level":"ERROR"
```

The alternatives were joined with `|` for the actual `rg -i` command. Before
use, the expression was sanity-checked against both archived beta.147 failure
logs. It matched the known issue #549 lines: `Service stop failed`,
`Error stopping services`, `graceful shutdown failed`, and
`heartbeat service not running`.

Applying the same expression to the qualifying `shutdown-window.log` returned
zero matches. All ten lines are INFO and the final line is
`SemStreams shutdown complete`. Independent container evidence records exit 0,
`OOMKilled=false`, an empty runtime error, requested signal 15, and no SIGKILL.
Silence is therefore not the sole basis for the pass.

Decision: pass. No issue #549 or genuine teardown failure signature occurred.
