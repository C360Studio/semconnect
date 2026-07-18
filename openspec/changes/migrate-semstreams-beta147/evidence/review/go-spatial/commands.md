# Go spatial review commands

Review completed at `2026-07-18T02:01:36Z` against signed source bundle
`7aff5daa1643bb2969c955a0f64ad9fa31f3b9bdb8bc9ab30de3b2c6e05e0a20`.

## Reviewer-found red and green

The first focused rerun captured the strict-array defect after the null cases
were added and before the decoder guard was present:

```text
$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go test -count=1 ./gateway/cs-api \
    -run 'TestClassifyEntityQueryFailure|TestHandleAreas_|TestLocationBuilders|TestMergePatchSystemTriples'
--- FAIL: TestHandleAreas_MalformedSuccessBodyBecomes500/null
    status: got 200 want 500; body={"type":"FeatureCollection","features":[]}
--- FAIL: TestHandleAreas_MalformedSuccessBodyBecomes500/___null__
    status: got 200 want 500; body={"type":"FeatureCollection","features":[]}
FAIL
```

After remediation, the exact focused command passed:

```text
$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go test -count=1 ./gateway/cs-api \
    -run 'TestClassifyEntityQueryFailure|TestHandleAreas_|TestLocationBuilders|TestMergePatchSystemTriples'
ok  github.com/c360studio/semconnect/gateway/cs-api  0.356s

$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go test -race -count=1 ./gateway/cs-api \
    -run 'TestClassifyEntityQueryFailure|TestHandleAreas_|TestLocationBuilders|TestMergePatchSystemTriples'
ok  github.com/c360studio/semconnect/gateway/cs-api  1.579s
```

## Repository Go gates without real-NATS listener tests

```text
$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go test -count=1 ./... -skip 'TestRealNATS'
all packages pass

$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go test -race -count=1 ./... -skip 'TestRealNATS'
all packages pass under the race detector

$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go vet ./...
[no output; exit 0]

$ GOCACHE=/tmp/semconnect-go-spatial-review-cache go build ./...
go: writing stat cache: open /Users/coby/go/pkg/mod/cache/download/github.com/c360studio/semconnect/@v/\
v0.0.0-20260706192504-a8d1a7d15a94.info126030281.tmp: operation not permitted
[exit 0; build completed, managed sandbox prevented only the nonessential module stat-cache update]
```

The root program manager separately reported fresh unsandboxed unfiltered
`go test ./...` and `go test -race ./...` passes. Those results are external
corroboration and are not represented as reviewer-executed commands.

## Contract, evidence, and formatting gates

```text
$ rg -n 'legacyPred|pre-beta|beta\.87|error: not found|TrimPrefix\(err\.Error\(\), "error: "' gateway/cs-api
[zero matches; rg exit 1]

$ openspec validate migrate-semstreams-beta147
Change 'migrate-semstreams-beta147' is valid

$ git diff --check -- gateway/cs-api/spatial.go gateway/cs-api/spatial_test.go \
    gateway/cs-api/spatial_projection_test.go gateway/cs-api/systems_post.go \
    gateway/cs-api/systems_patch.go gateway/cs-api/deployments_post.go \
    gateway/cs-api/sampling_features_post.go gateway/cs-api/systems.go \
    gateway/cs-api/systems_test.go \
    openspec/changes/migrate-semstreams-beta147/evidence/development/go-spatial
[no output; exit 0]

$ git diff --unified=1 -- gateway/cs-api/systems.go gateway/cs-api/systems_test.go | \
    rg -n '^[-+].*(conformance|conf/|skip|Skip|Claim|wantClaimed)'
[zero matches; rg exit 1]
```

The reviewer recomputed SHA-256 for all thirteen handoff files; every value
matches `spatial-development-handoff.json` and its aggregate source-bundle
identifier is `7aff5daa...e05e0a20`.

No external ETS run was performed by this reviewer, and this approval does
not authorize production cutover.
