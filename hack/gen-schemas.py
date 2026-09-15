#!/usr/bin/env python3
"""Extract standalone JSON Schemas from the generated CRDs.

Every CRD already carries an OpenAPI v3 schema, but only the API server sees it.
Writing it out as JSON Schema lets an editor validate a manifest before it is
applied, which is where a typo is cheapest to find.

Usage:
    hack/gen-schemas.py [--check]
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
CRD_DIR = ROOT / "config" / "crd" / "bases"
OUT_DIR = ROOT / "schemas"

# yaml-language-server matches a schema to a file by glob, but a reader matching
# by hand needs the kind and version in the filename too.
HEADER_NOTE = (
    "Generated from config/crd/bases by hack/gen-schemas.py. Do not edit. "
    "CEL rules under x-kubernetes-validations are NOT enforced by editors - "
    "only the API server evaluates them."
)


def schema_for(crd: dict, version: dict) -> dict:
    """Build a self-contained JSON Schema for one CRD version."""
    spec = crd["spec"]
    group, kind = spec["group"], spec["names"]["kind"]
    api_version = f"{group}/{version['name']}"

    body = version["schema"]["openAPIV3Schema"]
    properties = dict(body.get("properties") or {})

    # The API server infers these; a standalone schema has to state them, and
    # pinning them by const is what lets an editor tell one kind from another.
    properties["apiVersion"] = {
        "type": "string",
        "const": api_version,
        "description": f"APIVersion of this resource. Always {api_version}.",
    }
    properties["kind"] = {
        "type": "string",
        "const": kind,
        "description": f"Kind of this resource. Always {kind}.",
    }
    properties.setdefault("metadata", {"type": "object"})

    return {
        "$schema": "http://json-schema.org/draft-07/schema#",
        "$id": f"https://rashkash103.github.io/authentik-operator/schemas/{kind.lower()}-{version['name']}.json",
        "title": f"{kind} ({api_version})",
        "description": (body.get("description") or f"{kind} resource.").strip(),
        "x-generated-by": HEADER_NOTE,
        "type": "object",
        "required": ["apiVersion", "kind", "spec"],
        "properties": properties,
        # Reject unknown top-level keys: a misspelled `specs:` is exactly the
        # mistake this is meant to catch before apply.
        "additionalProperties": False,
    }


def render() -> dict[str, str]:
    """Return the desired contents of every schema file, keyed by filename."""
    out: dict[str, str] = {}
    for path in sorted(CRD_DIR.glob("*.yaml")):
        with path.open() as handle:
            crd = yaml.safe_load(handle)
        kind = crd["spec"]["names"]["kind"]
        for version in crd["spec"]["versions"]:
            name = f"{kind.lower()}-{version['name']}.json"
            out[name] = json.dumps(schema_for(crd, version), indent=2, sort_keys=False) + "\n"
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    desired = render()
    if not desired:
        print(f"ERROR: no CRDs found in {CRD_DIR}", file=sys.stderr)
        return 1

    OUT_DIR.mkdir(exist_ok=True)
    existing = {p.name: p.read_text() for p in OUT_DIR.glob("*.json")}

    if existing == desired:
        print(f"{len(desired)} schemas are up to date")
        return 0

    if args.check:
        stale = sorted(set(desired) ^ set(existing)) or [
            n for n in desired if existing.get(n) != desired[n]
        ]
        print(f"ERROR: schemas are out of date ({', '.join(stale)}); run 'make schemas' and commit",
              file=sys.stderr)
        return 1

    for name in set(existing) - set(desired):
        (OUT_DIR / name).unlink()
    for name, body in desired.items():
        (OUT_DIR / name).write_text(body)
    print(f"wrote {len(desired)} schemas to {OUT_DIR.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
