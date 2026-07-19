# Beta.149 pin alignment

Final values:

- Go module: `github.com/c360studio/semstreams v1.0.0-beta.149`.
- Conformance version: `v1.0.0-beta.149`.
- Conformance commit: `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`.
- Commit date: `2026-07-18`.
- Checked-out tag and peeled commit both resolve to the same commit.
- `go.sum` contains exactly the beta.149 module and `go.mod` checksums; it contains no checksum for an older
  SemStreams version.

The guard parses active `go.mod` requirements, ignores commented text, rejects duplicate module requirements, parses
all shell-effective `.ets-pin` assignments, and rejects duplicate assignments that could override the intended pin.

Final uncached tests:

```console
$ go test ./conformance -run '^(TestSemStreamsPinsAreAligned|TestActiveModuleRequirementsRejectTextualFalsePositives|TestShellAssignmentsPreserveDuplicateEffectivePins)$' -count=1 -v
=== RUN   TestSemStreamsPinsAreAligned
=== PAUSE TestSemStreamsPinsAreAligned
=== RUN   TestActiveModuleRequirementsRejectTextualFalsePositives
=== PAUSE TestActiveModuleRequirementsRejectTextualFalsePositives
=== RUN   TestShellAssignmentsPreserveDuplicateEffectivePins
=== PAUSE TestShellAssignmentsPreserveDuplicateEffectivePins
=== CONT  TestSemStreamsPinsAreAligned
=== CONT  TestShellAssignmentsPreserveDuplicateEffectivePins
--- PASS: TestShellAssignmentsPreserveDuplicateEffectivePins (0.00s)
=== CONT  TestActiveModuleRequirementsRejectTextualFalsePositives
--- PASS: TestActiveModuleRequirementsRejectTextualFalsePositives (0.00s)
--- PASS: TestSemStreamsPinsAreAligned (0.00s)
PASS
ok      github.com/c360studio/semconnect/conformance  0.221s
```

File hashes after final guard hardening at `2026-07-18T10:39:30Z`:

```text
a41ce208147b015cd6e93d048481b847f6bee2b12d439bc8fb4196107d3fc530  go.mod
6fb8194f5b78f830fed0fd8c28c9308d910ffe94d6cd50968128f92860a1960a  go.sum
bf219bfdb0845b01ed4c8f6429280f39d8b2e75a51c3dd42d64fb17bb54e3ba0  conformance/.ets-pin
35a080307d8cf8b6dc598f681d40ae7ccd5e8be523e57f00c28e94b5cdd1c0ef  conformance/pin_alignment_test.go
```

No product compatibility code was added.
