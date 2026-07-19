# beta.147-only source audit

Command:

```sh
rg -n 'legacyPred|pre-beta|beta\.87|error: not found|TrimPrefix\(err\.Error\(\), "error: "' gateway/cs-api
```

Result: zero matches.

The current SemStreams integration contains no pre-beta predicate alias or
body-string error fallback. The two SensorML HTTP media types remain on
purpose: they are external CS API representation parity, not a SemStreams
state compatibility path.

The spatial write projection uses only these public beta.147 constants:

- `vocabulary.GeoLocationLongitude`
- `vocabulary.GeoLocationLatitude`
- `vocabulary.GeoLocationAltitude`

The owned `sensorml.process.position` triple remains the lossless CS API /
SensorML geometry representation; it is not treated as an input alias by
graph-index-spatial.
