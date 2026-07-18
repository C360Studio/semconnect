# Go readiness review commands

Review completed at `2026-07-18T01:37:03Z` against signed source bundle
`0af3071091e32ab0c60e9a0ff1068c4a9fadc5ebbbab8109b8700ace19ef5710`.

## Focused Go verification

```text
$ GOCACHE=/tmp/semconnect-go-review-cache go test -count=1 ./conformance/cmd/index-readiness
ok  github.com/c360studio/semconnect/conformance/cmd/index-readiness  0.210s

$ GOCACHE=/tmp/semconnect-go-review-cache go test -race -count=1 ./conformance/cmd/index-readiness
ok  github.com/c360studio/semconnect/conformance/cmd/index-readiness  1.314s

$ GOCACHE=/tmp/semconnect-go-review-cache go vet ./conformance/cmd/index-readiness
[no output; exit 0]
```

## Shell, OpenSpec, and diff verification

```text
$ bash -n conformance/run.sh
[no output; exit 0]

$ openspec validate migrate-semstreams-beta147
Change 'migrate-semstreams-beta147' is valid

$ git diff --check -- conformance/run.sh conformance/fixtures/system.sml.json \
    conformance/cmd/index-readiness \
    openspec/changes/migrate-semstreams-beta147/evidence/operations/readiness-development \
    openspec/changes/migrate-semstreams-beta147/evidence/operations/retained-state-identity-impact.md \
    openspec/changes/migrate-semstreams-beta147/evidence/operations/conformance-cutover-manifest.blocked.json
[no output; exit 0]
```

## Signed-handoff hash comparison

```text
$ shasum -a 256 conformance/run.sh conformance/fixtures/system.sml.json \
    conformance/cmd/index-readiness/main.go \
    conformance/cmd/index-readiness/main_test.go \
    conformance/cmd/index-readiness/seed_identity_test.go
5342d3d76cb5941fbf6fa69ea9ffd371df492df4f7344e26900cb11626a743a3  conformance/run.sh
e2c08a0146abf15c30c186a23980eb5611193a58f827d0bf0a9a2246c74ef018  conformance/fixtures/system.sml.json
d1e6f01c99b87e60d354a26b84e372a411fa4753073c59e0e06d6fc75cf8e5bc  conformance/cmd/index-readiness/main.go
7d391116dc22264e0d8b786e8d9993fc27a1d08d807fe19880c1df44f8ed1b55  conformance/cmd/index-readiness/main_test.go
ecbdc145a6d597252517198488be9968b47c3a1cac7838f7d26ae85a094ce6b0  conformance/cmd/index-readiness/seed_identity_test.go
```

The hashes exactly match `readiness-development-handoff.json`. The retained-state report and blocked manifest also
reference the same `0af307...` source bundle.

No Docker/Team Engine run was performed by this reviewer. This review therefore does not claim the external
`137 passed / 0 failed / 0 skipped` gate and does not authorize a production cutover.
