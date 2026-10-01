#!/usr/bin/env python3
"""Assert the canonical deployment fixture's real-backend spatial contract."""

import argparse
import json
from pathlib import Path
import sys
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import urlopen


SYSTEM_ID = "c360.semconnect.systems.csapi.system.v1"
POINT = {"type": "Point", "coordinates": [-96.797, 32.777]}


def probe(base_url, output):
    output.mkdir(parents=True, exist_ok=True)
    polygon = {
        "type": "Polygon",
        "coordinates": [[[-97, 32], [-96, 32], [-96, 33], [-97, 33], [-97, 32]]],
    }
    cases = [
        ("bbox-hit", {"bbox": "-97,32,-96,33"}, True),
        ("polygon-hit", {"polygon": json.dumps(polygon, separators=(",", ":"))}, True),
        ("bbox-miss", {"bbox": "10,10,11,11"}, False),
    ]
    summary = {}
    failures = []
    for name, params, hit in cases:
        url = base_url.rstrip("/") + "/areas?" + urlencode(params)
        try:
            response = urlopen(url, timeout=10)
        except HTTPError as error:
            response = error
        with response:
            body = response.read()
            status = response.status
            headers = dict(response.headers.items())
        (output / (name + ".body.json")).write_bytes(body)
        (output / (name + ".http.json")).write_text(
            json.dumps({"url": url, "status": status, "headers": headers}, indent=2) + "\n"
        )
        try:
            document = json.loads(body)
            assert status == 200, f"HTTP status {status}"
            assert document.get("type") == "FeatureCollection", "expected FeatureCollection"
            features = document.get("features")
            assert isinstance(features, list), "features must be an array"
            if hit:
                assert len(features) == 1, f"expected one canonical feature, got {len(features)}"
                feature = features[0]
                assert feature.get("type") == "Feature", "expected Feature"
                assert feature.get("id") == SYSTEM_ID, f"unexpected entity {feature.get('id')}"
                assert feature.get("geometry") == POINT, f"unexpected point {feature.get('geometry')}"
                assert feature.get("properties", {}).get("href") == "/systems/" + SYSTEM_ID
            else:
                assert features == [], "remote bbox unexpectedly returned entities"
            summary[name] = {"status": status, "response": document}
        except (AssertionError, json.JSONDecodeError) as error:
            failures.append(f"{name}: {error}")
    (output / "summary.json").write_text(json.dumps(summary, sort_keys=True, indent=2) + "\n")
    (output / "result.json").write_text(
        json.dumps({"passed": len(cases) - len(failures), "failed": len(failures), "failures": failures}, indent=2)
        + "\n"
    )
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print(f"PASS: {len(cases)} spatial cases; evidence: {output}")
    return 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    sys.exit(probe(args.url, args.output))
