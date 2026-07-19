# Retained-state identity-impact report

- **Deployment**: disposable `semconnect-conformance` rehearsal
- **Identity decision**: no retained-reference change in the signed seed contract
- **Runtime decision**: disposable restart identity/readability proof passed
- **Production decision**: no-go pending manifest rehearsal and approvals
- **Observation stream**: `CS_API_OBSERVATIONS`
- **Artifact ObjectStore**: `CS_API_ARTIFACTS`
- **Source handoff**:
  `readiness-development/readiness-development-handoff.json`

## Why identity stability matters

The observation subject embeds the Datastream entity ID. Schema ObjectStore
keys embed schema artifact IDs, and graph artifact entities must be rebuilt
with those exact identities after the graph-state wipe. A changed parent,
artifact, subject, or key would leave retained data unreachable through the CS
API even if the bytes remained in JetStream.

## Signed stable identities

For the long artifact values below, concatenate the two adjacent code
fragments with no whitespace or separator.

- System source UID, the deterministic source for the System ID:
  `urn:ets:system:weather-station-01`
- System entity, stable across reseed:
  `c360.semconnect.systems.csapi.system.weather-station-01`
- Datastream entity, stable retained observation parent:
  `c360.semconnect.systems.csapi.datastream.weather-temperature-01`
- Result-schema artifact, stable graph relationship target:
  `c360.semconnect.systems.csapi.schema.`
  `c360_semconnect_systems_csapi_datastream_weather-temperature-01-resultSchema`
- Result-schema ObjectStore key:
  `c360.semconnect.systems.csapi.schema.`
  `c360_semconnect_systems_csapi_datastream_weather-temperature-01-resultSchema.json`
- Observation subject, stable retained JetStream subject:
  `cs-api.observations.c360.semconnect.systems.csapi.datastream.weather-temperature-01`
- Observation payload ID, stable within the retained subject:
  `ets-observation-001`
- SystemEvent entity, stable seed/query identity:
  `c360.semconnect.systems.csapi.systemevent.00Z`
- ControlStream entity, stable command-schema parent:
  `c360.semconnect.systems.csapi.controlstream.ptz-01`
- Command-schema artifact, stable graph relationship target:
  `c360.semconnect.systems.csapi.schema.`
  `c360_semconnect_systems_csapi_controlstream_ptz-01-commandSchema`
- Command-schema ObjectStore key:
  `c360.semconnect.systems.csapi.schema.`
  `c360_semconnect_systems_csapi_controlstream_ptz-01-commandSchema.json`

The focused seed audit derives the values twice, rejects UUID fallback, and
validates each graph ID with the beta.147 canonical validator. `run.sh` also
fails if POST responses return a different System, Datastream, ControlStream,
or SystemEvent ID.

## Developer proof and review approval

The refreshed developer handoff names both ObjectStore objects and attests
repeat derivation, canonical validation, and no UUID fallback. Its source
bundle checksum is
`0af3071091e32ab0c60e9a0ff1068c4a9fadc5ebbbab8109b8700ace19ef5710`.

Independent Go review is approved with no findings at
`../review/go-readiness/approval.json`. The reviewer matched every file
hash to the signed handoff, reran focused tests, race, vet, shell syntax,
OpenSpec, and diff checks, and approved both retained schema objects. The
blocked manifest therefore records `identityImpact=none` for the observation
stream and artifact store.

The disposable run `2026-07-18T02-03-23Z` then proved stable stream/ObjectStore
counts, readable observation and result-schema responses, and identical
normalized response hashes across a no-write restart. Detailed runtime
evidence is in `rehearsal/retained-state-proof.json` and
`rehearsal/replay-parity.json`.

## Production proof still required

The disposable evidence is necessary but does not substitute for the
deployment-specific cutover. Production must still record:

- exact pre-cutover observation message and ObjectStore object counts;
- the actual subjects, object keys, and byte hashes present before deletion;
- identical subjects, keys, and readable payloads after canonical reseed;
- rebuilt artifact entity IDs and parent relationships;
- observation and schema reads before and after the no-write restart.

The disposable run did not capture per-object raw ObjectStore byte hashes.
Any production mismatch is a no-go and requires a separate preservation or
retirement plan. No blanket stream or ObjectStore deletion is approved.
