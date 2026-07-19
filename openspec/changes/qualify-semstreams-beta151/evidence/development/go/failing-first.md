# Failing-first record

The beta.151 alignment test was changed before any pin. Against the beta.149
working state, this command failed as required:

```console
$ go test ./conformance -run TestSemStreamsPinsAreAligned -count=1
--- FAIL: TestSemStreamsPinsAreAligned
    go.mod active requirements = ["v1.0.0-beta.149"], want beta.151
    go.sum contains beta.149 and lacks beta.151 checksums
    SEMSTREAMS_TAG_OBJECT was absent
    SEMSTREAMS_COMMIT was 7db0cdcb..., want ac75c322...
    SEMSTREAMS_VERSION was beta.149, want beta.151
FAIL
```

Exit code: `1`.

The source-identity test was also written before the harness correction. It
demonstrated that the beta.149 harness reused a vendor checkout solely because
`HEAD` matched, even if tracked or untracked bytes differed:

```console
$ go test ./conformance -run \
  'TestHarnessVendorReuseRequiresExactCleanSource|TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift' \
  -count=1
--- FAIL: TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
    stat conformance/lib/vendor_identity.sh: no such file or directory
--- FAIL: TestHarnessVendorReuseRequiresExactCleanSource
    ensure_semstreams_vendor does not bind vendor reuse to exact clean source identity
FAIL
```

Exit code: `1`. This is source-integrity TDD, not compatibility behavior.

Before external qualification, the reviewer found the same HEAD-only reuse
hazard on the separately materialized ETS checkout. The harness-specific test
was written before changing `ensure_ets_vendor` and failed against the existing
implementation:

```console
$ go test ./conformance \
  -run TestHarnessETSVendorReuseRequiresExactCleanSource -count=1
--- FAIL: TestHarnessETSVendorReuseRequiresExactCleanSource (0.00s)
    vendor_identity_test.go:28: ensure_ets_vendor does not bind vendor reuse to exact clean source identity
FAIL
```

Exit code: `1`. The test scopes its assertions to the
`ensure_ets_vendor` function and requires clean pinned reuse, exact-path
validation, isolated materialization, clean-source verification before
replacement, and movement of the intact Git checkout. The shared helper test
already supplies tracked, untracked, and `git status` failure cases; this red
test proves the ETS harness path had not applied that protection.

Beta.149 commit `7db0cdcb...` also contains none of the following beta.151
proof symbols/files:

```text
UnmarshalEntityStateTrusted
graph/entity_state_trusted_decode_test.go
processor/graph-ingest/structural_predicate_gate_test.go
processor/graph-ingest/structural_gate_wire_integration_test.go
processor/graph-ingest/rmw_trusted_decode_integration_test.go
```

The absence was checked with `git grep` and `git ls-tree` against the exact
beta.149 commit. This records task 3.1 without modifying the accepted beta.149
evidence.
