# Focused upstream PR #550 lifecycle verification

Source checkout:

```text
tag: v1.0.0-beta.149
commit: 7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2
working tree: clean
```

Command run from `conformance/.vendor/semstreams`:

```console
$ go test ./service -run '^(TestHeartbeatService_(StopNotRunning|StopIdempotent|StartAfterStop|ContextCancellation)|TestMetricsForwarder_(StopBeforeStart|StopIdempotent|StopAfterContextCancellation|StartAfterStop)|TestServiceManager_StopAll_(Idempotency|CancellationBeforeStopAll)|TestBaseService_StopIdempotent)$' -count=1 -v
```

Passing tests:

```text
TestHeartbeatService_StopNotRunning
TestHeartbeatService_StopIdempotent
TestHeartbeatService_StartAfterStop
TestHeartbeatService_ContextCancellation
TestMetricsForwarder_StopBeforeStart
TestMetricsForwarder_StopIdempotent
TestMetricsForwarder_StopAfterContextCancellation
TestMetricsForwarder_StartAfterStop
TestServiceManager_StopAll_Idempotency/already-stopped_service_is_not_aggregated_as_fatal
TestServiceManager_StopAll_Idempotency/genuine_stop_failure_is_surfaced_and_others_still_stopped
TestServiceManager_StopAll_Idempotency/fully_clean_shutdown_returns_nil
TestServiceManager_StopAll_CancellationBeforeStopAll
TestBaseService_StopIdempotent
```

Final output:

```text
PASS
ok      github.com/c360studio/semstreams/service  0.268s
```

The intentional `genuine_stop_failure_is_surfaced_and_others_still_stopped` subtest logs a synthetic `boom` error to
prove real failures remain fatal. It passed and is not an issue #549 signature. Coverage includes heartbeat and
metrics-forwarder teardown, repeated Stop, cancellation before `StopAll`, BaseService terminal-state draining, and
rejection of Start on spent service instances. This suite was rerun after final `go mod tidy` normalization.

