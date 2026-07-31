# Greenfield Compose deployment

This bundle starts the complete pre-v1 semconnect product surface: NATS with JetStream, SemStreams beta.159, and the
semconnect CS API. It is for a new NATS volume only. It has no state-import, upgrade, or old-state path.

The beta.159 bundle passed clean-volume startup, canonical query readiness, normal stop, same-volume restart,
persistence parity, and unchanged external `137/0/0`. The observations stream is bounded at 1 GiB/30 days with
`DiscardOld`. The bundle is production-ready for this greenfield pre-v1 scope; it does not claim in-place startup
over a beta.153 volume.

NATS is internal-only on the private Compose network and publishes no host ports, so it has no NATS credentials or
secret inputs. Publishing NATS outside that network is a different security design and is not
covered by this bundle. The CS API is published on `${SEMCONNECT_PORT:-8080}`.

## Start

From the repository root:

```sh
docker compose -f deploy/compose.yml up -d --build
```

The named `semconnect-nats-data` volume retains JetStream state across ordinary Compose stops, starts, and host
restarts. Do not point this pre-v1 bundle at an existing NATS namespace or volume.

## First-start and persistence proof

The checked-in verifier proves that NATS has zero streams before SemStreams starts, seeds one versioned System,
waits for collection/query readiness, stops all three services normally, restarts them over the same volume, and
compares the normalized query proof byte-for-byte:

```sh
SEMCONNECT_NATS_VOLUME=semconnect-beta159-rehearsal \
  EVIDENCE_DIR=/tmp/semconnect-beta159-evidence \
  ./deploy/verify-persistence.sh
```

The verifier deliberately leaves services and the named volume in place for operator inspection. It never removes
storage. A clean NATS stop is proven from its JetStream shutdown lifecycle log and `OOMKilled=false`; the NATS image
reports signal exit 1 after that complete shutdown, while SemStreams and semconnect must both exit zero. The canonical
fixture is [canonical-system.v1.json](canonical-system.v1.json).
