# beta.147 spatial wire capture

Captured from the disposable beta.147 rehearsal stack on 2026-07-18 UTC by
requesting the live NATS subject directly.

## Successful bounds response

Subject: `graph.spatial.query.bounds`

Request:

```json
{"north":0.00001,"south":0,"east":0.00001,"west":0,"limit":100}
```

Response headers: none.

Exact response body:

```json
[]
```

This confirms the current success contract remains a bare JSON array of the
public graph-index-spatial `SpatialResult` shape.

## Classified bounds failure

Subject: `graph.spatial.query.bounds`

Request:

```json
{"north":90,"south":-90,"east":180,"west":-180,"limit":100}
```

Exact relevant response headers:

```text
X-Status: error
X-Error-Class: transient
```

Exact response body:

```json
{"message":"Component.handleQueryBoundsNATS: list spatial index keys failed: nats: no keys found"}
```

The rehearsal 500 was therefore not a success-envelope incompatibility. The
gateway used the byte-only `Request` method, discarded ADR-060 headers, and
attempted to decode the classified error object as `[]spatialResult`.
