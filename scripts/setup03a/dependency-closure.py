#!/usr/bin/env python3
"""Measure SemConnect's consumer and retained SemStreams backend import closures.

Does not execute tests, fetch a newer revision, or edit module pins. Raw go-list
records stay outside committed evidence; normalized package sets are reviewable.
"""

import argparse
import datetime
import json
import pathlib
import re
import subprocess

FRAMEWORK = "github.com/c360studio/semstreams"
BACKEND_ROOTS = [FRAMEWORK + "/" + path for path in (
    "component", "config", "service", "message", "natsclient", "payloadregistry",
    "payloadbuiltins", "processor/graph-ingest", "processor/graph-index",
    "processor/graph-index-spatial", "processor/graph-index-temporal",
    "processor/graph-query", "storage/objectstore",
)]
# Tags supplied by the Go toolchain or platform selection are not custom test tags.
BUILTIN_TAGS = set("ignore cgo race msan asan unix gc gccgo amd64 arm64 arm 386 ppc64 ppc64le riscv64 s390x mips mipsle mips64 mips64le wasm loong64 aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows".split())


def stream_json(value):
    decoder = json.JSONDecoder()
    offset = 0
    while offset < len(value):
        while offset < len(value) and value[offset].isspace():
            offset += 1
        if offset == len(value):
            break
        record, offset = decoder.raw_decode(value, offset)
        yield record


def package_key(record):
    key = record.get("ForTest") or record["ImportPath"].split(" [", 1)[0]
    # go-list emits a synthetic test main in addition to the real package.
    if key.endswith(".test") and record.get("Name") == "main":
        return None
    return key


def normalized(records):
    result = {}
    for record in records:
        key = package_key(record)
        if key is not None:
            result[key] = record
    return result


def test_tags(packages):
    tags = set()
    constraints = {}
    for name, record in packages.items():
        if not name.startswith(FRAMEWORK) and not record.get("Module", {}).get("Main"):
            continue
        directory = pathlib.Path(record["Dir"])
        for path in directory.glob("*_test.go"):
            for line in path.read_text().splitlines()[:30]:
                if line.startswith("//go:build "):
                    expression = line.removeprefix("//go:build ")
                    constraints[f"{name}/{path.name}"] = expression
                    for tag in re.findall(r"[A-Za-z_][A-Za-z_0-9.]*", expression):
                        if tag not in BUILTIN_TAGS and not tag.startswith("go1."):
                            tags.add(tag)
    return sorted(tags), constraints


def summarize(packages):
    groups = {"framework": [], "consumer": [], "third_party": [], "standard": []}
    lines = 0
    for name, record in sorted(packages.items()):
        if name == FRAMEWORK or name.startswith(FRAMEWORK + "/"):
            category = "framework"
            for path in pathlib.Path(record["Dir"]).glob("*.go"):
                if not path.name.endswith("_test.go"):
                    lines += len(path.read_text().splitlines())
        elif record.get("Module", {}).get("Main"):
            category = "consumer"
        elif record.get("Standard"):
            category = "standard"
        else:
            category = "third_party"
        groups[category].append(name)
    return {"counts": {k: len(v) for k, v in groups.items()},
            "framework_non_test_source_lines": lines, "packages": groups}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--raw-output", type=pathlib.Path, required=True)
    parser.add_argument("--label", required=True)
    parser.add_argument("--source-commit", help="Explicit source commit for an extracted Git archive without .git")
    parser.add_argument("--replay-raw", action="store_true", help="Re-normalize captured output without running Go")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    args.raw_output.mkdir(parents=True, exist_ok=True)
    commands = []

    def run(name, argv):
        commands.append({"name": name, "argv": argv})
        if args.replay_raw:
            return (args.raw_output / (name + ".stdout")).read_text()
        result = subprocess.run(argv, text=True, capture_output=True, check=False)
        (args.raw_output / (name + ".stdout")).write_text(result.stdout)
        (args.raw_output / (name + ".stderr")).write_text(result.stderr)
        if result.returncode:
            raise RuntimeError(f"{name} failed ({result.returncode}): {result.stderr}")
        return result.stdout

    try:
        environment = json.loads(run("go-env", ["go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED"]))
        module = json.loads(run("framework-module", ["go", "list", "-mod=readonly", "-m", "-json", FRAMEWORK]))
        metadata = {
            "label": args.label,
            "captured_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
            "consumer_commit": args.source_commit or run("consumer-commit", ["git", "rev-parse", "HEAD"]).strip(),
            "consumer_status": (["Git archive; supplemental closure measurement script supplied"] if args.source_commit
                                else run("consumer-status", ["git", "status", "--short"]).splitlines()),
            "framework_module": {k: module[k] for k in ("Path", "Version", "Sum", "GoModSum") if k in module},
            "environment": environment,
            "method": "Host GOOS/GOARCH. Production package closure; all retained framework packages' tests, union of default, custom tags, and race+custom tags. Lists imports only; does not execute tests. Raw non-test source lines include inactive source files in each retained framework directory.",
        }
        scopes = {}
        production_by_scope = {}
        for scope, roots in (("consumer", ["./..."]), ("backend_composed", BACKEND_ROOTS)):
            production_records = list(stream_json(run(scope + "-production", ["go", "list", "-mod=readonly", "-deps", "-json", *roots])))
            production = normalized(production_records)
            production_by_scope[scope] = production
            retained = sorted(name for name in production if name.startswith(FRAMEWORK + "/"))
            tests_roots = roots if scope == "consumer" else retained
            tags, constraints = test_tags(production)
            combined = dict(production)
            variants = [("default", []), ("custom", tags), ("race-custom", ["race", *tags])]
            for variant, selected in variants:
                argv = ["go", "list", "-mod=readonly", "-deps", "-test", "-json"]
                if selected:
                    argv += ["-tags=" + ",".join(selected)]
                records = list(stream_json(run(scope + "-tests-" + variant, [*argv, *tests_roots])))
                combined.update(normalized(records))
            direct = sorted({imp for record in production_records if not record.get("DepOnly") for imp in record.get("Imports", []) if imp.startswith(FRAMEWORK + "/")})
            scopes[scope] = {"roots": roots, "direct_framework_imports": direct,
                             "retained_framework_test_roots": retained if scope != "consumer" else [],
                             "custom_test_tags": tags, "build_constraints": constraints,
                             "production": summarize(production), "with_retained_tests": summarize(combined),
                             "test_only_framework_packages": sorted(set(combined).difference(production).intersection(name for name in combined if name.startswith(FRAMEWORK + "/")))}

        # The actual consumer can import framework packages absent from the
        # stable backend roots. Their transitive tests belong to extraction
        # planning as well; retaining only backend tests understates closure.
        production = {**production_by_scope["consumer"], **production_by_scope["backend_composed"]}
        retained = sorted(name for name in production if name.startswith(FRAMEWORK + "/"))
        tags, constraints = test_tags(production)
        combined = dict(production)
        for variant, selected in [("default", []), ("custom", tags), ("race-custom", ["race", *tags])]:
            argv = ["go", "list", "-mod=readonly", "-deps", "-test", "-json"]
            if selected:
                argv += ["-tags=" + ",".join(selected)]
            records = stream_json(run("combined-tests-" + variant, [*argv, *retained]))
            combined.update(normalized(records))
        scopes["combined"] = {
            "roots": retained,
            "note": "Union of actual consumer and stable backend production closures; tests enumerate every retained framework package.",
            "retained_framework_test_roots": retained,
            "custom_test_tags": tags, "build_constraints": constraints,
            "production": summarize(production), "with_retained_tests": summarize(combined),
            "test_only_framework_packages": sorted(name for name in combined if name.startswith(FRAMEWORK + "/") and name not in production),
        }
        shipped = normalized(stream_json(run("stock-backend-production", ["go", "list", "-mod=readonly", "-deps", "-json", FRAMEWORK + "/cmd/semstreams"])))
        scopes["stock_backend_binary"] = {"roots": [FRAMEWORK + "/cmd/semstreams"], "production": summarize(shipped), "note": "Actual beta.160 backend is the full upstream binary; this scope is a comparison point, not the bounded retained extraction set. Target uses a consumer-owned backend whose imports are in the consumer scope."}
        metadata["scopes"] = scopes
        (args.output / "closure.json").write_text(json.dumps(metadata, indent=2) + "\n")
        for scope, report in scopes.items():
            for variant in ("production", "with_retained_tests"):
                if variant not in report:
                    continue
                (args.output / f"{scope}-{variant}.txt").write_text("\n".join(report[variant]["packages"]["framework"]) + "\n")
        print(json.dumps({scope: {variant: report[variant]["counts"] for variant in ("production", "with_retained_tests") if variant in report} for scope, report in scopes.items()}, indent=2))
    finally:
        (args.output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")


if __name__ == "__main__":
    main()
