# Notice of modifications — OGC API Connected Systems OAS3 source

Per the terms of the OGC License (`LICENSE-OGC.txt`, paragraph 2):
> *If you modify the Intellectual Property, all copies of the modified
> Intellectual Property must include … a notice that the Intellectual
> Property includes modifications that have not been approved or
> adopted by LICENSOR.*

## Modifications

The OAS3 source files in this directory (`part1/`, `part2/`, `common/`)
are **unmodified** copies of the upstream source at
[`opengeospatial/ogcapi-connected-systems`](https://github.com/opengeospatial/ogcapi-connected-systems)
commit `3fd86c73e744b7e2faaf7f1c17366bfb9ff4cd6f`.

The cs-api-server *served* OpenAPI document (`../openapi.yaml`) is a
**separate, hand-authored work** adapted from this source. It includes
schema overrides reflecting cs-api's actual v0.1 behavior:

- Honest `X-CS-*` response headers for documented deferrals
  (`X-CS-Reconstructed-Lossy`, `X-CS-Geometry-Available`,
  `X-CS-Datastream-Subset`).
- `x-not-implemented-at-v01: true` extension on paths deferred to
  follow-up stages, with pointers back into this directory.
- Local server URL block.
- Conformance class declarations reflecting what cs-api actually
  claims (not the full Part 1 + Part 2 set).

**These adaptations have not been approved or adopted by OGC.** They
exist only to make the cs-api gateway's OAS3 surface honest about its
v0.1 scope, and should not be construed as proposed normative changes
to the OGC standards.

## Reporting issues

Issues with the *upstream* OGC OAS source should be filed at
<https://github.com/opengeospatial/ogcapi-connected-systems/issues>.

Issues with the *cs-api adaptation* (`../openapi.yaml`) should be filed
at <https://github.com/C360Studio/semconnect/issues>.
