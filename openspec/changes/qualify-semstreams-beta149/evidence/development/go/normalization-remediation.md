# Go module normalization remediation

Independent review found that a post-attestation module-list diagnostic had added this unrelated checksum after the
original Go developer handoff:

```text
github.com/google/go-tpm-tools v0.3.13-0.20230620182252-4639ecce2aba/go.mod
```

The implementation/evidence gate was reopened. `go mod tidy` was run from the repository root and completed with
exit code `0` and no output. The normalized `go.sum` has 238 lines, contains exactly the beta.149 SemStreams module
and `go.mod` checksum entries, and contains no `google/go-tpm-tools` entry. Its final SHA-256 is:

```text
6fb8194f5b78f830fed0fd8c28c9308d910ffe94d6cd50968128f92860a1960a  go.sum
```

Relative to the accepted beta.147 baseline, the normalized file replaces two beta.147 checksums with two beta.149
checksums and removes 192 unused historical checksum lines. No dependency requirement changed beyond the intended
SemStreams version pin.

After normalization, the uncached alignment test, focused upstream lifecycle suite, and the complete task 2.5 matrix
were rerun. Their refreshed results are in `pin-alignment.md`, `upstream-lifecycle.md`, and `verification.md`.

