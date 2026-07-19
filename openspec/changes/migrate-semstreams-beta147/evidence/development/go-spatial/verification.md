# Spatial migration verification

Verified at `2026-07-18T01:53:05Z`.

Reviewer remediation reverified at `2026-07-18T01:58:28Z`.

## Passing gates

```text
GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -run 'TestHandleAreas_' -count=1
ok github.com/c360studio/semconnect/gateway/cs-api

GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -run 'TestClassifyEntityQueryFailure|TestHandleAreas_|TestLocationBuilders|TestMergePatchSystemTriples' \
  -count=1
ok github.com/c360studio/semconnect/gateway/cs-api

GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -skip 'TestRealNATS' -count=1
ok github.com/c360studio/semconnect/gateway/cs-api

GOCACHE=/private/tmp/semconnect-gocache go test ./... \
  -skip 'TestRealNATS' -count=1
all packages pass

GOCACHE=/private/tmp/semconnect-gocache go build ./...
pass (rerun outside the managed filesystem sandbox so Go could update its
module stat cache)

GOCACHE=/private/tmp/semconnect-gocache go vet ./...
pass

git diff --check
pass
```

## Managed-sandbox limitation

An unfiltered `go test ./gateway/cs-api -count=1` reached only these two
failures:

```text
TestRealNATSEntityMutationCarriesCanonicalFinalState: NATS server did not become ready
TestRealNATSJetStreamAndObjectStoreLifecycle: NATS server did not become ready
```

All non-real-NATS tests passed. This is the known managed-sandbox listener
restriction, not a product assertion failure. The root/operator must run the
unfiltered real-NATS gate outside the sandbox.

## External root corroboration

After this implementation, the root program-manager agent reported that its
unsandboxed `go test ./...` and `go test -race ./...` runs both passed,
including the real-NATS tests. These are recorded as external corroboration;
the spatial developer agent does not claim to have executed them.

## Deliberately not run

No external ETS/conformance run, stack restart, reseed, or production mutation
was performed by this developer task.
