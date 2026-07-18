#!/usr/bin/env python3
"""Validate operations JSON with the JSON Schema keywords used by ADR-S003.

This bounded validator uses only the Python standard library. It intentionally
fails on an unsupported schema keyword so validation cannot silently weaken as
the architecture schema evolves.
"""

from __future__ import annotations

import json
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any


ANNOTATIONS = {"$schema", "$id", "title", "description"}
SUPPORTED = {
    "$ref",
    "type",
    "additionalProperties",
    "required",
    "properties",
    "const",
    "enum",
    "allOf",
    "if",
    "then",
    "$defs",
    "items",
    "minItems",
    "maxItems",
    "contains",
    "minContains",
    "minLength",
    "pattern",
    "not",
    "minimum",
    "format",
}


def resolve(root: dict[str, Any], reference: str) -> dict[str, Any]:
    if not reference.startswith("#/"):
        raise ValueError(f"unsupported non-local reference: {reference}")
    value: Any = root
    for token in reference[2:].split("/"):
        value = value[token.replace("~1", "/").replace("~0", "~")]
    if not isinstance(value, dict):
        raise ValueError(f"reference does not resolve to a schema: {reference}")
    return value


def is_type(value: Any, expected: str) -> bool:
    return {
        "object": isinstance(value, dict),
        "array": isinstance(value, list),
        "string": isinstance(value, str),
        "boolean": isinstance(value, bool),
        "integer": isinstance(value, int) and not isinstance(value, bool),
    }.get(expected, False)


def validate(
    schema: dict[str, Any],
    value: Any,
    root: dict[str, Any],
    path: str = "$",
) -> list[str]:
    unknown = set(schema) - SUPPORTED - ANNOTATIONS
    if unknown:
        return [f"{path}: unsupported schema keywords: {sorted(unknown)}"]
    if "$ref" in schema:
        return validate(resolve(root, schema["$ref"]), value, root, path)

    errors: list[str] = []
    expected_type = schema.get("type")
    if expected_type and not is_type(value, expected_type):
        return [f"{path}: expected {expected_type}, got {type(value).__name__}"]
    if "const" in schema and value != schema["const"]:
        errors.append(f"{path}: expected constant {schema['const']!r}")
    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path}: value {value!r} is not in {schema['enum']!r}")

    for branch in schema.get("allOf", []):
        errors.extend(validate(branch, value, root, path))
    if "if" in schema and not validate(schema["if"], value, root, path):
        errors.extend(validate(schema.get("then", {}), value, root, path))

    if isinstance(value, dict):
        required = schema.get("required", [])
        for name in required:
            if name not in value:
                errors.append(f"{path}: missing required property {name!r}")
        properties = schema.get("properties", {})
        if schema.get("additionalProperties") is False:
            for name in value.keys() - properties.keys():
                errors.append(f"{path}: unexpected property {name!r}")
        for name, child_schema in properties.items():
            if name in value:
                errors.extend(validate(child_schema, value[name], root, f"{path}.{name}"))

    if isinstance(value, list):
        if len(value) < schema.get("minItems", 0):
            errors.append(f"{path}: has {len(value)} items, below minimum")
        if "maxItems" in schema and len(value) > schema["maxItems"]:
            errors.append(f"{path}: has {len(value)} items, above maximum")
        if "items" in schema:
            for index, item in enumerate(value):
                errors.extend(validate(schema["items"], item, root, f"{path}[{index}]"))
        if "contains" in schema:
            matches = sum(not validate(schema["contains"], item, root, path) for item in value)
            if matches < schema.get("minContains", 1):
                errors.append(f"{path}: contains matched {matches} items")

    if isinstance(value, str):
        if len(value) < schema.get("minLength", 0):
            errors.append(f"{path}: string is shorter than minLength")
        if "pattern" in schema and not re.search(schema["pattern"], value):
            errors.append(f"{path}: string does not match {schema['pattern']!r}")
        if schema.get("format") == "date-time":
            try:
                datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                errors.append(f"{path}: invalid date-time {value!r}")

    if isinstance(value, int) and not isinstance(value, bool):
        if "minimum" in schema and value < schema["minimum"]:
            errors.append(f"{path}: integer is below minimum")
    if "not" in schema and not validate(schema["not"], value, root, path):
        errors.append(f"{path}: value matches forbidden schema")
    return errors


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: validate-json-schema.py SCHEMA MANIFEST", file=sys.stderr)
        return 2
    schema = json.loads(Path(sys.argv[1]).read_text())
    manifest = json.loads(Path(sys.argv[2]).read_text())
    errors = validate(schema, manifest, schema)
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"schema valid: {sys.argv[2]}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
