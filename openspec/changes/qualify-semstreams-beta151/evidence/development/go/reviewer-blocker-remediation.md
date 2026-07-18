# Reviewer blocker B151-GO-REV-001 remediation

The reviewer correctly found that `git_source_matches_commit` discarded a
nonzero `git status` exit with `|| true`. If status produced no stdout, the
helper could interpret failure as a clean source tree and reuse it.

Failing-first reproduction preserved a valid commit and corrupted only the Git
index. `git rev-parse HEAD` therefore still succeeded while `git status` failed:

```console
$ go test ./conformance \
  -run TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift \
  -count=1 -v
=== RUN   TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
    vendor_identity_test.go:64: git_source_matches_commit(...) success = true, want false
--- FAIL: TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
FAIL
```

The helper now captures `git status` in an `if ! ...; then return 1; fi`
condition. It preserves the command's exit status and fails closed before
examining stdout. No compatibility or product behavior changed.

Final focused result:

```console
$ go test ./conformance \
  -run 'TestHarnessVendorReuseRequiresExactCleanSource|\
TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift|\
TestSemStreamsPinsAreAligned' -count=1 -v
--- PASS: TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
--- PASS: TestHarnessVendorReuseRequiresExactCleanSource
--- PASS: TestSemStreamsPinsAreAligned
PASS
```

The complete unit, race, integration, default vet, integration vet, build, and
tidy-diff matrix was rerun after remediation and passed. Production authority
remains withheld.

## Pre-external ETS source-integrity hardening

The subsequent reviewer check found that `ensure_ets_vendor` still reused the
Botts ETS checkout when `HEAD` alone matched `ETS_COMMIT`. Dirty tracked,
untracked, ignored, or submodule bytes could therefore enter the TeamEngine
image while the summary continued to report the pinned commit.

The failing-first harness assertion is archived in `failing-first.md`.
`ensure_ets_vendor` now calls the same fail-closed
`git_source_matches_commit` helper used for SemStreams. On mismatch it:

1. validates the exact harness-owned `conformance/.vendor/ets` target;
2. performs a non-shallow `--filter=blob:none` clone into an isolated
   `.ets-refresh.*` directory;
3. checks out and validates the pinned commit before touching the active path;
4. moves the complete checkout, including `.git` history and SCM metadata,
   into place only after validation.

The clone remains a real Git repository because the ETS Maven
`buildnumber-maven-plugin` consumes SCM metadata. Neither the ETS pin nor any
suite, fixture, OpenAPI, conformance claim, filter, skip, parser, or product
behavior changed.

Focused result:

```console
$ go test ./conformance \
  -run 'TestHarness(ETS)?VendorReuseRequiresExactCleanSource|\
TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift' \
  -count=1 -v
--- PASS: TestGitSourceMatchesCommitRejectsTrackedAndUntrackedDrift
--- PASS: TestHarnessVendorReuseRequiresExactCleanSource
--- PASS: TestHarnessETSVendorReuseRequiresExactCleanSource
PASS
```

The complete local qualification matrix was rerun after this hardening and
passed. No external conformance run began before the source-integrity gate was
closed. Production authority remains withheld.
