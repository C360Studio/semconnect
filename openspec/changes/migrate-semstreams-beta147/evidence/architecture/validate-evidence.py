#!/usr/bin/env python3
"""Validate the frozen architecture evidence without third-party dependencies."""

from __future__ import annotations

import hashlib
import json
import pathlib
import re
import sys


COMMIT = "c8f0b92edf5ad5b491d5f4e81891bec817fae3cd"
TREES = (
    "message/oms",
    "parser/sensorml",
    "pkg/swecommon",
    "vocabulary/csapi",
    "vocabulary/oms",
    "vocabulary/sosa",
    "vocabulary/swe",
)
SHA256 = re.compile(r"^[0-9a-f]{64}$")
TASK_MARKER = re.compile(r"(?m)^- \[[ xX]\](?= \d+\.\d+ )")
HERE = pathlib.Path(__file__).resolve().parent
REPOSITORY = HERE.parents[4]


def fail(message: str) -> None:
    raise SystemExit(f"architecture evidence invalid: {message}")


def load_json(path: pathlib.Path) -> dict:
    try:
        with path.open("r", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as error:
        fail(f"cannot load {path}: {error}")


def digest(path: pathlib.Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def validate_provenance(source_root: pathlib.Path | None) -> None:
    manifest = load_json(HERE / "provenance-manifest.json")
    if manifest.get("sourceCommit") != COMMIT:
        fail("provenance commit differs from the approved pre-deletion revision")
    if manifest.get("treeCount") != 7 or manifest.get("fileCount") != 55:
        fail("provenance count is not seven trees and 55 files")
    if tuple(item["source"] for item in manifest.get("trees", [])) != TREES:
        fail("provenance tree set or order changed")

    files = manifest.get("files", [])
    sources = [item.get("source") for item in files]
    destinations = [item.get("destination") for item in files]
    if len(files) != 55 or len(set(sources)) != 55 or len(set(destinations)) != 55:
        fail("provenance files are missing or duplicated")
    if sources != sorted(sources) or sources != destinations:
        fail("provenance paths are not sorted or destination topology changed")

    for item in files:
        if item.get("mode") not in {"100644", "100755"}:
            fail(f"invalid Git mode for {item.get('source')}")
        if not SHA256.fullmatch(str(item.get("sha256", ""))):
            fail(f"invalid SHA-256 for {item.get('source')}")
        if not isinstance(item.get("bytes"), int) or item["bytes"] < 0:
            fail(f"invalid byte count for {item.get('source')}")

    if source_root is None:
        return
    actual_paths = sorted(
        path.relative_to(source_root).as_posix()
        for tree in TREES
        for path in (source_root / tree).rglob("*")
        if path.is_file()
    )
    if actual_paths != sources:
        fail("source checkout file set differs from provenance manifest")
    by_source = {item["source"]: item for item in files}
    for relative in actual_paths:
        path = source_root / relative
        item = by_source[relative]
        mode = "100755" if path.stat().st_mode & 0o111 else "100644"
        if mode != item["mode"] or path.stat().st_size != item["bytes"] or digest(path) != item["sha256"]:
            fail(f"source checkout differs from provenance manifest at {relative}")


def validate_ledger() -> None:
    ledger = load_json(HERE / "semantic-ledger.json")
    spec_path = REPOSITORY / ledger["sourceSpec"]["path"]
    if digest(spec_path) != ledger["sourceSpec"]["sha256"]:
        fail("semantic ledger source-spec checksum changed")
    groups = (
        ("transferredRenames", 19),
        ("localRenames", 12),
        ("fullIriCorrections", 1),
    )
    for name, count in groups:
        if len(ledger.get(name, [])) != count:
            fail(f"semantic ledger {name} count differs from {count}")
    if ledger.get("counts", {}).get("totalCorrections") != 32:
        fail("semantic ledger total differs from 32")
    if any(value != "forbidden" for key, value in ledger.get("disposition", {}).items() if key != "migration"):
        fail("semantic ledger permits a compatibility disposition")
    if ledger.get("disposition", {}).get("migration") != "authoritative-reseed":
        fail("semantic ledger migration is not authoritative reseed")


def validate_cutover_contract() -> None:
    schema = load_json(HERE / "cutover-manifest.schema.json")
    template = load_json(HERE / "cutover-manifest.template.json")
    required = set(schema.get("required", []))
    if required != set(template):
        fail("cutover template does not contain exactly the top-level required fields")
    if template.get("template") is not True or template.get("immutable") is not False:
        fail("cutover template is not an editable draft")
    if template.get("destructiveScope") or template.get("approvals"):
        fail("cutover template contains destructive scope or approvals")
    if template.get("execution") != {"authorized": False, "goNoGoDecision": "pending"}:
        fail("cutover template authorizes execution")
    gate = template.get("p0Gate", {})
    if gate.get("status") != "blocked" or not gate.get("blockingConditions"):
        fail("cutover template does not preserve the explicit P0 block")
    schema_text = json.dumps(schema, sort_keys=True)
    for token in (
        "deploymentValues",
        "destructiveScope",
        "retainedState",
        "authoritativeSource",
        "rollback",
        "approvals",
        "minContains",
    ):
        if token not in schema_text:
            fail(f"cutover schema omits required execution gate {token}")


def validate_task_contract() -> None:
    snapshot = load_json(HERE / "task-contract.snapshot.json")
    task_path = REPOSITORY / snapshot["source"]
    live = task_path.read_text(encoding="utf-8")
    task_count = len(TASK_MARKER.findall(live))
    normalized = TASK_MARKER.sub("- [ ]", live).encode("utf-8")
    expected = snapshot.get("normalization", {}).get("normalizedTaskDefinitionSha256")
    if snapshot.get("taskCount") != task_count:
        fail(f"live task count {task_count} differs from signed task contract")
    if not SHA256.fullmatch(str(expected)) or hashlib.sha256(normalized).hexdigest() != expected:
        fail("live task definition drifted; checkbox-only progression is the sole permitted change")


def validate_checksums_and_handoff() -> None:
    checksums_path = HERE / "evidence-checksums.sha256"
    entries: dict[str, str] = {}
    for line in checksums_path.read_text(encoding="utf-8").splitlines():
        expected, relative = line.split("  ", 1)
        if not SHA256.fullmatch(expected) or relative in entries:
            fail(f"invalid checksum entry: {line}")
        entries[relative] = expected
        if digest(REPOSITORY / relative) != expected:
            fail(f"checksum mismatch: {relative}")
    live_tasks = "openspec/changes/migrate-semstreams-beta147/tasks.md"
    snapshot = "openspec/changes/migrate-semstreams-beta147/evidence/architecture/task-contract.snapshot.json"
    if live_tasks in entries or snapshot not in entries:
        fail("checksum trust boundary must sign the task-contract snapshot, not live task state")
    handoff = load_json(HERE / "architecture-handoff.json")
    signature = handoff.get("signature", {})
    if signature.get("signedByRole") != "architect" or signature.get("decision") != "approved-for-implementation":
        fail("architecture handoff is not signed for implementation")
    if signature.get("evidenceBundleSha256") != digest(checksums_path):
        fail("architecture handoff signature does not cover the evidence bundle")
    if handoff.get("productionGate", {}).get("status") != "P0-BLOCKED":
        fail("architecture handoff does not block production cutover")
    remediation = load_json(HERE / "architecture-remediation-handoff.json")
    remediation_signature = remediation.get("signature", {})
    if remediation.get("reviewFinding") != "GO-REV-004":
        fail("architecture remediation handoff does not identify GO-REV-004")
    if remediation_signature.get("decision") != "ready-for-reviewer-rereview":
        fail("architecture remediation is not signed for reviewer rereview")
    if remediation_signature.get("evidenceBundleSha256") != digest(checksums_path):
        fail("architecture remediation signature does not cover the evidence bundle")


def main() -> None:
    if len(sys.argv) > 2:
        fail("usage: validate-evidence.py [SEMSTREAMS_SOURCE_ROOT]")
    source_root = pathlib.Path(sys.argv[1]).resolve() if len(sys.argv) == 2 else None
    validate_provenance(source_root)
    validate_ledger()
    validate_cutover_contract()
    validate_task_contract()
    validate_checksums_and_handoff()
    print(
        "architecture evidence valid: 7 trees, 55 files, 32 semantic corrections, "
        "58 stable task definitions, P0 production block"
    )


if __name__ == "__main__":
    main()
