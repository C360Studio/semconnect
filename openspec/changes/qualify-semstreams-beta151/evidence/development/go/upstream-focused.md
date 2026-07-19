# Focused upstream verification

All commands ran from the clean beta.151 source at `ac75c322...`.

PR #554 default and live-wire tests passed:

```console
$ go test ./processor/graph-ingest -run '<structural gate tests>' -count=1 -v
PASS
$ go test -tags=integration ./processor/graph-ingest \
  -run 'TestIntegration_(StructuralGate|SharedSeam_ClaimedForeignEdge)' -count=1 -v
PASS
```

Coverage includes triple add, batch add, create-with-triples,
update-with-triples, direct ingest, classified NATS replies, valid persistence,
whole-batch rejection, and claimed NoBirthStub routing.

PR #567 trusted-decode and RMW tests passed:

```console
$ go test ./graph -run TestUnmarshalEntityStateTrusted -count=1 -v
PASS
$ go test -tags=integration ./processor/graph-ingest \
  -run 'TestIntegration_(RMWResidentPoison|RemoveTriple_NoOp|RMWTrustedDecode)' -count=1 -v
PASS
```

PR #561's cancellation regression passed:

```console
$ go test ./processor/rule \
  -run TestEvaluatePersistsMatchStateWhenCtxCancelledMidAction -count=1 -v
PASS
```

Graph-index authoritative synchronization passed for both codecs, including
seed throughput, exact queries, churn convergence, restart parity, and resource
checks:

```console
$ go test -tags=integration ./processor/graph-index \
  -run TestIntegration_PredicateLayoutSmoke -count=1 -v
PASS
```

The inherited beta.149 lifecycle regressions also passed: idempotent service
manager and BaseService stop, MetricsForwarder repeated stop, and stop after
context cancellation. The intentional synthetic `boom` subtest remained fatal,
proving real shutdown errors are not swallowed.
