# Technical-writer verification

Verification completed on 2026-07-18 for the pre-rehearsal evidence envelope.

## Schema and specification

```text
$ python3 evidence/operations/validate-json-schema.py \
    evidence/architecture/cutover-manifest.schema.json \
    evidence/operations/conformance-cutover-manifest.blocked.json
schema valid: .../conformance-cutover-manifest.blocked.json

$ jq empty evidence/operations/*.json evidence/review/go-readiness/*.json
[no output; exit 0]

$ openspec validate migrate-semstreams-beta147 --strict
Change 'migrate-semstreams-beta147' is valid

$ python3 evidence/architecture/validate-evidence.py /private/tmp/semstreams-c8f0b92-review
architecture evidence valid: 7 trees, 55 files, 32 semantic corrections,
58 stable task definitions, P0 production block
```

The command paths above are abbreviated from
`openspec/changes/migrate-semstreams-beta147/`. The bounded standard-library
validator rejects any schema keyword it does not implement, so a future schema
expansion cannot silently bypass validation.

## Readiness increment

```text
$ bash -n conformance/run.sh
[no output; exit 0]

$ GOCACHE=/private/tmp/semconnect-technical-writer-gocache \
    go test -count=1 ./conformance/cmd/index-readiness
ok github.com/c360studio/semconnect/conformance/cmd/index-readiness 0.273s

$ GOCACHE=/private/tmp/semconnect-technical-writer-gocache \
    go vet ./conformance/cmd/index-readiness
[no output; exit 0]
```

The first local attempt without an explicit `GOCACHE` was blocked by managed
sandbox permissions on the host Go cache. The temp-cache rerun above is the
fresh result. Independent reviewer commands, including focused race, are at
`../review/go-readiness/commands.md`.

## Documentation and evidence integrity

```text
$ git diff --check
[no output; exit 0]

$ awk 'length($0) > 120 ...' evidence/operations/*.md ...
[no output; exit 0 for the new operations and transfer documents]

$ rg '2d316380|b8695da3|identityImpact=review-required' evidence/operations \
    -g '!technical-writer-verification.md'
[no output; exit 1 means no stale evidence pointers]
```

Local documentation targets referenced by the handoff, identity report,
operations index, ADR amendments, README, and conformance guide all exist.
Production execution, Docker rehearsal, TestNG, live resource counts, and
replay parity were intentionally not claimed by this verification.

## Disposable runtime closeout

The later run `2026-07-18T02-03-23Z` supplies the runtime claims that were
intentionally absent above. The technical-writer closeout verified:

- TestNG XML SHA-256 and exact `137/0/0` attributes;
- readiness JSONL SHA-256 values and revisions `80/80`, `118/118`, and
  `118/118`;
- restart-log SHA-256, line count, two shutdown ERROR records, and successful
  backend/query bootstrap;
- independent task 9.5 approval and its anti-weakening evidence hashes;
- exact full image IDs supplied by the program-manager capture;
- all eleven full normalized query SHA-256 pairs;
- the archived 11-file pre/post corpus and empty recursive diff;
- archived post-restart readiness `118/118` and its SHA-256;
- JSON syntax, OpenSpec strict validation, documentation line length, and
  `git diff --check`.

The runtime closeout does not revise the earlier non-claims retroactively.
Its evidence lives in `rehearsal/`, and its production decision is no-go.
