#!/usr/bin/env python3
"""Sanity-check the generated CRD schemas.

controller-gen accepts markers that produce schemas which are syntactically
valid but reject every value. The checks here are for mistakes that pass
generation silently and only surface when a user's `kubectl apply` is refused.

Usage:
    hack/check-crds.py
"""

from __future__ import annotations

import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
CRD_DIR = ROOT / "config" / "crd" / "bases"


def walk(schema: dict, path: str, problems: list[str]) -> None:
    """Recurse a JSON schema, recording anything suspicious."""
    if not isinstance(schema, dict):
        return

    # An enum on an array constrains the whole list, so no non-empty value can
    # ever satisfy it. It happens when an enum marker is put on a []T field
    # instead of on a named element type.
    if schema.get("type") == "array" and "enum" in schema:
        problems.append(
            f"{path}: enum is on the array itself, so every non-empty value is rejected. "
            f"Move the marker onto a named element type."
        )

    for key in ("properties", "patternProperties"):
        for name, child in (schema.get(key) or {}).items():
            walk(child, f"{path}.{name}", problems)

    if "items" in schema:
        walk(schema["items"], f"{path}[]", problems)

    # A required field that also has a default is contradictory: the default
    # can never apply.
    required = set(schema.get("required") or [])
    for name, child in (schema.get("properties") or {}).items():
        if name in required and isinstance(child, dict) and "default" in child:
            problems.append(f"{path}.{name}: is required but also has a default, so the default is unreachable")


def main() -> int:
    files = sorted(CRD_DIR.glob("*.yaml"))
    if not files:
        print(f"no CRDs found in {CRD_DIR}", file=sys.stderr)
        return 1

    problems: list[str] = []
    for path in files:
        with path.open() as handle:
            doc = yaml.safe_load(handle)
        kind = doc["spec"]["names"]["kind"]
        for version in doc["spec"]["versions"]:
            schema = version.get("schema", {}).get("openAPIV3Schema")
            if schema:
                walk(schema, f"{kind}/{version['name']}", problems)

    for problem in problems:
        print(f"ERROR: {problem}", file=sys.stderr)

    if problems:
        return 1
    print(f"checked {len(files)} CRDs, no schema problems found")
    return 0


if __name__ == "__main__":
    sys.exit(main())
