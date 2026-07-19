# Pin and effective-source alignment

Final identities:

- module: `github.com/c360studio/semstreams v1.0.0-beta.151`;
- annotated tag object: `784f22dc8d549d7781b88a2878bb679112aad494`;
- peeled commit: `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`;
- source tree: `120eeb353afb7d07aa1b3180de05f75494bac1a8`;
- commit date: `2026-07-18`;
- source repository: `https://github.com/C360Studio/semstreams.git`.

`go.sum` contains exactly the beta.151 module and `go.mod` entries for
SemStreams. `conformance/.ets-pin` records the repository, version, tag object,
peeled commit, and date as single effective assignments.

The conformance harness now treats Git `HEAD` as necessary but insufficient.
It reuses a vendor tree only when the commit matches and `git status` reports no
tracked, untracked, ignored, or submodule drift. A mismatch is refreshed into a
temporary directory, verified, and moved into the one exact harness-owned
`conformance/.vendor/semstreams` target. This prevents a dirty checkout from
producing an image whose source bytes are not bound to the pin.

Reviewer blocker B151-GO-REV-001 additionally proved that `git status` itself
must succeed. A corrupt-index fixture preserves a resolvable `HEAD` but makes
status fail; source matching now returns false on that command failure.

The same exact-source contract now protects the pinned Botts ETS checkout.
`ensure_ets_vendor` reuses only a clean checkout at `ETS_COMMIT`; any drift or
status failure triggers materialization in an isolated temporary directory.
The harness validates the exact destination before replacement and moves the
whole non-shallow partial clone so `.git` history and Maven SCM metadata remain
available. `ETS_COMMIT` remains `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`.

Final uncached guard:

```console
$ go test ./conformance \
  -run 'Test(SemStreamsPins|ActiveModule|ShellAssignments|HarnessVendor|GitSource)' \
  -count=1 -v
--- PASS: TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
--- PASS: TestShellAssignmentsPreserveDuplicateEffectivePins
--- PASS: TestActiveModuleRequirementsRejectTextualFalsePositives
--- PASS: TestHarnessVendorReuseRequiresExactCleanSource
--- PASS: TestHarnessETSVendorReuseRequiresExactCleanSource
--- PASS: TestSemStreamsPinsAreAligned
PASS
```

The materialized SemStreams source was clean at `ac75c322...` when focused
upstream tests ran.
