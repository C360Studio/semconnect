# Downstream structural and trusted-RMW proof

`TestBeta151StructuralMutationContract` starts SemStreams' standard NATS 2.12
JetStream testcontainer, binds semconnect's actual System projection contract,
starts beta.151 graph-ingest, and communicates only through production NATS
mutation subjects.

It proved:

- the real `system-hosted.sml.json` projection creates a canonical parent;
- its foreign-subject `sensorml.component.is-hosted-by` edge is claimed,
  materializes an envelope-bearing child stub, and is stored on that child;
- a canonical update preserves the exact subject, predicate, and object;
- malformed predicate, entity ID, and entity reference updates return the
  invalid classification and leave parent bytes, entity revision, and whole KV
  bucket revision unchanged;
- adversarial raw resident poison reaches the trusted owner-RMW read but the
  authoritative write gate returns `graph_state_reset_required` without changing
  bytes, entity revision, or bucket revision;
- removing a valid but absent predicate is a true no-op with no KV write.

Command:

```console
$ go test -tags=integration ./gateway/cs-api \
  -run TestBeta151StructuralMutationContract -count=1 -v
--- PASS: TestBeta151StructuralMutationContract
    --- PASS: reject_invalid_predicate_atomically
    --- PASS: reject_invalid_entity_ID_atomically
    --- PASS: reject_invalid_entity_reference_atomically
    --- PASS: resident_poison_is_not_laundered
    --- PASS: remove_no-op_does_not_write
PASS
```

The final test was also run with `-count=3`; all three independent NATS
containers and contract passes succeeded.

The test deliberately creates ownership buckets and binds the projection before
graph-ingest starts. This ensures the positive foreign-edge case exercises the
real `Contract -> Bind -> ClaimReader -> NoBirthStub` lane, not graph-ingest's
claim-reader-unavailable fallback.
