# Pin-alignment failing-first record

Recorded at `2026-07-18T10:25Z` before changing `go.mod`, `go.sum`, or
`conformance/.ets-pin`.

Command:

```console
$ go test ./conformance -run '^TestSemStreamsPinsAreAligned$' -count=1 -v
=== RUN   TestSemStreamsPinsAreAligned
=== PAUSE TestSemStreamsPinsAreAligned
=== CONT  TestSemStreamsPinsAreAligned
    pin_alignment_test.go:29: go.mod does not require "github.com/c360studio/semstreams v1.0.0-beta.149"
    pin_alignment_test.go:36: go.sum does not contain checksum entry beginning "github.com/c360studio/semstreams v1.0.0-beta.149 "
    pin_alignment_test.go:36: go.sum does not contain checksum entry beginning "github.com/c360studio/semstreams v1.0.0-beta.149/go.mod "
    pin_alignment_test.go:47: conformance/.ets-pin does not contain exact line "SEMSTREAMS_VERSION=v1.0.0-beta.149"
    pin_alignment_test.go:47: conformance/.ets-pin does not contain exact line "SEMSTREAMS_COMMIT=7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2"
    pin_alignment_test.go:47: conformance/.ets-pin does not contain exact line "SEMSTREAMS_COMMIT_DATE=2026-07-18"
--- FAIL: TestSemStreamsPinsAreAligned (0.00s)
FAIL
FAIL    github.com/c360studio/semconnect/conformance  0.240s
FAIL
```

Exit code: `1`. The failure covered every pin surface required by task 2.1.

