# Readiness remediation TDD record

This record distinguishes retained command evidence from bounded reconstruction. It does not claim an unexecuted red
test or invent timestamps/output that were not retained.

## Initial readiness helper: retained red run

Before `main.go` existed, `go test ./conformance/cmd/index-readiness` failed to compile. The exact wall-clock timestamp
was not retained. The captured failure named the missing contract seams:

```text
undefined: waitConfig
undefined: waitForReadiness
undefined: evidenceEvent
FAIL github.com/c360studio/semconnect/conformance/cmd/index-readiness [build failed]
```

The first implementation then made the focused package green.

## Advanced current target: bounded reconstruction

The go-reviewer found that the pre-remediation success expression accepted this status:

```text
captured target = 10
current target  = 12
indexed         = 10
ready           = true
```

The exact red-test command output and finding timestamp were not retained because the focused test and Boolean guard
were applied together. This is recorded as a sequencing miss, not as a claimed red run. The failure is mechanically
reconstructable from the prior predicate: current target was at least the captured target, and indexed revision only
had to cover the captured target. Both comparisons were true for the status above.

The remediation adds `TestWaitForReadinessRequiresCoverageOfAdvancedCurrentTarget`. It proves the helper issues a
fourth request and waits for indexed revision 12. Success now explicitly requires indexed revision to cover both the
captured and current target revisions.

## Missing ControlStream command-schema artifact: bounded reconstruction

The technical-writer found that the first seed audit covered only the Datastream result-schema object. No failing
test output was retained before the ControlStream coverage was added. The gap is reconstructable from the first
signed source bundle: `seed_identity_test.go` hash `02c9163f...` had no ControlStream or command-schema assertions,
and `run.sh` hash `54bfe796...` did not carry an explicit ControlStream ID or command-schema key drift check.

The remediation now parses the actual ControlStream JSON seed, pins its exact entity ID, derives the command-schema
artifact twice, validates the entity IDs, rejects UUID fallback, and asserts the exact `.json` ObjectStore key.

## Current green evidence

At `2026-07-18T01:33:39Z`:

```text
$ GOCACHE=/private/tmp/semconnect-gocache go test ./conformance/cmd/index-readiness
ok github.com/c360studio/semconnect/conformance/cmd/index-readiness 0.285s
```

This run also includes the exact `ets-observation-001` payload-ID assertion and both retained schema-object keys.
