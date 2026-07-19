# Spatial migration TDD record

## Failing first: response classification and geo projection

Command:

```sh
GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -run 'TestHandleAreas_(ClassifiedBackendErrors|MalformedSuccessBodyBecomes500)|TestLocationBuilders|TestMergePatchSystemTriples' \
  -count=1
```

Observed red before production edits:

- classified Invalid and Transient replies returned HTTP 500 instead of 400
  and 503 because the error body was decoded as a success array;
- System Feature, System SensorML, Deployment Feature, and Sampling Feature
  builders emitted no `geo.location.longitude`, `geo.location.latitude`, or
  `geo.location.altitude` triples;
- System PATCH retained the old 10/20/30 spatial projection after replacing
  geometry with 40/50/60;
- replacement with a Polygon retained stale Point projection predicates.

## Failing first: code-only not-found classification

Command:

```sh
GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -run TestClassifyEntityQueryFailure -count=1
```

Observed red before production edit:

```text
uncoded_not_found_text_remains_Invalid: probe failed:
cs-api: entity not found: not found: acme.ops.robotics.gcs.drone.999
```

The old body-text parser incorrectly promoted an uncoded Invalid error to the
local 404 sentinel.

## Green implementation

- `runSpatialQuery` now uses `RequestWithHeaders` and
  `natsclient.ClassifyReply`; success remains strict bare-array decoding.
- Point geometry is preserved under `sensorml.process.position` and projected
  separately to beta.147 public geo vocabulary constants.
- Non-Point and malformed geometry never produce invented coordinates.
- PATCH geometry replacement removes the full prior geo projection before
  appending a new Point projection; 2D/non-Point replacements cannot retain a
  stale altitude/coordinate.
- Entity not-found maps to 404 only when the beta.147 public
  `graph.ErrorCodeEntityNotFound` code is present.

Both focused command sets pass after implementation.

## Reviewer remediation: JSON null is not a success array

Failing-first command:

```sh
GOCACHE=/private/tmp/semconnect-gocache go test ./gateway/cs-api \
  -run TestHandleAreas_MalformedSuccessBodyBecomes500 -count=1
```

Observed red before the decoder edit:

```text
TestHandleAreas_MalformedSuccessBodyBecomes500/null:
status: got 200 want 500; body={"type":"FeatureCollection","features":[]}

TestHandleAreas_MalformedSuccessBodyBecomes500/___null__:
status: got 200 want 500; body={"type":"FeatureCollection","features":[]}
```

Go accepts JSON `null` when unmarshalling into a slice, producing a nil slice
without error. The beta.147 contract requires a top-level JSON array, so the
decoder now trims JSON whitespace and rejects any success payload whose first
token is not `[`. This is a strict contract gate, not a compatibility branch.

The exact test passes after implementation.
