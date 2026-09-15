#!/usr/bin/env python3
"""Report authentik API changes that would break this operator's decoding.

The operator is pinned to one authentik series because `goauthentik.io/api/v3`
is generated from one release and enforces *that* release's required properties
when decoding. A field authentik adds and marks required makes every list call
against older releases fail with `no value given for required property`. That is
how 2026.5 and 2026.2 broke: `Application.pbm_uuid` is required in 2026.8 and
absent in both.

Nothing warned about it. The E2E matrix found it by failing.

This compares authentik's published OpenAPI schema between two series and
reports only the changes that can break us, on only the schemas this operator
actually touches. Against 2026.5 -> 2026.8 that is one finding out of 925
component schemas, which is a signal worth acting on rather than a report to
skim.

It is deliberately not part of `just verify`: it needs the network, and the
verify contract has to work offline. Run it when authentik cuts a release, or
let the scheduled workflow run it.

Usage:
    hack/check-api-drift.py                      # pinned series vs authentik main
    hack/check-api-drift.py --candidate 2026.9   # pinned series vs a named series
    hack/check-api-drift.py --baseline 2026.2 --candidate 2026.8
    hack/check-api-drift.py --list-schemas       # what is considered in scope
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys
import urllib.error
import urllib.request

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
VERSIONS = ROOT / "supported-versions.yaml"
CACHE = ROOT / "bin" / "schema-cache"

# authentik publishes the schema on a branch per series; `main` is the unreleased
# tip. The branch is `version-2026.8`, with a hyphen.
SCHEMA_URL = "https://raw.githubusercontent.com/goauthentik/authentik/{ref}/schema.yml"


def pinned_series() -> str:
    with VERSIONS.open() as fh:
        return str(yaml.safe_load(fh)["maximum"])


def load_spec(source: str) -> dict:
    """Load a spec by series name, `main`, or local path."""
    path = pathlib.Path(source)
    if path.exists():
        with path.open() as fh:
            return yaml.safe_load(fh)

    ref = source if source == "main" else f"version-{source}"
    CACHE.mkdir(parents=True, exist_ok=True)
    cached = CACHE / f"{ref}.yml"

    if not cached.exists():
        url = SCHEMA_URL.format(ref=ref)
        try:
            with urllib.request.urlopen(url, timeout=60) as resp:  # noqa: S310
                cached.write_bytes(resp.read())
        except urllib.error.HTTPError as exc:
            if exc.code == 404:
                sys.exit(f"ERROR: no schema at {url}\n"
                         f"       {source!r} is not a published authentik series")
            raise
    with cached.open() as fh:
        return yaml.safe_load(fh)


def referenced_types() -> set[str]:
    """Type names the operator mentions, from the Go source.

    Derived rather than listed, so a new adapter cannot quietly fall outside the
    check. Names that are not component schemas - the API groups, the client
    plumbing - drop out when this is intersected with the spec.
    """
    out: set[str] = set()
    for path in ROOT.rglob("*.go"):
        if "vendor" in path.parts:
            continue
        for name in re.findall(r"\bapi\.([A-Z][A-Za-z0-9]+)", path.read_text()):
            out.add(name)
            # Constructors: api.NewApplicationRequest -> ApplicationRequest.
            if name.startswith("New") and len(name) > 3 and name[3].isupper():
                out.add(name[3:])
    return out


def in_scope(spec: dict) -> set[str]:
    """Every schema the operator decodes or sends, including nested ones.

    A required property added to a *nested* schema breaks decoding exactly as
    one added to the top level does, so the root set is closed over $ref.
    """
    schemas = spec["components"]["schemas"]
    roots = {n for n in referenced_types() if n in schemas}
    # List endpoints return a paginated envelope around the model.
    roots |= {f"Paginated{n}List" for n in list(roots) if f"Paginated{n}List" in schemas}

    seen: set[str] = set()
    stack = list(roots)
    while stack:
        name = stack.pop()
        if name in seen or name not in schemas:
            continue
        seen.add(name)
        for ref in re.findall(r"#/components/schemas/([A-Za-z0-9_]+)",
                              yaml.safe_dump(schemas[name])):
            if ref not in seen:
                stack.append(ref)
    return seen


def compare(old: dict, new: dict, names: set[str]) -> list[str]:
    """Changes that can break decoding a response or sending a request."""
    a = old["components"]["schemas"]
    b = new["components"]["schemas"]
    findings: list[str] = []

    for name in sorted(names):
        before, after = a.get(name), b.get(name)
        if before is None:
            continue  # new schema; nothing we send or decode yet depends on it
        if after is None:
            findings.append(f"{name}: schema removed")
            continue

        bprops, aprops = before.get("properties", {}), after.get("properties", {})
        breq, areq = set(before.get("required", [])), set(after.get("required", []))

        # The pbm_uuid case: required in the newer release, so a client generated
        # from it cannot decode a response from the older one.
        for field in sorted(areq - breq):
            where = "added" if field not in bprops else "existing"
            findings.append(
                f"{name}.{field}: newly required ({where} field) - a client "
                f"generated from the newer spec cannot decode the older response")

        # A property that disappears breaks a client that still requires it.
        for field in sorted(set(bprops) - set(aprops)):
            if field in breq:
                findings.append(
                    f"{name}.{field}: removed, and the older spec requires it - "
                    f"a client generated from the older spec cannot decode the newer response")

        # Enum narrowing: a value we may still send is no longer accepted.
        for field in sorted(set(bprops) & set(aprops)):
            gone = set(bprops[field].get("enum", [])) - set(aprops[field].get("enum", []))
            if gone:
                findings.append(f"{name}.{field}: enum values removed -> {sorted(gone)}")

    return findings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--baseline", help="series, 'main', or a path (default: the pinned series)")
    parser.add_argument("--candidate", default="main",
                        help="series, 'main', or a path (default: main)")
    parser.add_argument("--list-schemas", action="store_true",
                        help="print the in-scope schemas and exit")
    args = parser.parse_args()

    baseline = args.baseline or pinned_series()

    old = load_spec(baseline)
    new = load_spec(args.candidate)
    names = in_scope(new) | in_scope(old)

    if args.list_schemas:
        print(f"{len(names)} schemas in scope, of "
              f"{len(new['components']['schemas'])} in the spec:")
        for n in sorted(names):
            print(f"  {n}")
        return 0

    findings = compare(old, new, names)

    print(f"authentik {baseline} -> {args.candidate}: "
          f"{len(names)} schemas in scope of "
          f"{len(new['components']['schemas'])} in the spec")

    if not findings:
        print("no decode-breaking changes found")
        print("\nThis says the shapes are compatible, not that the behaviour is.")
        print("Confirm by running the E2E suite against the candidate.")
        return 0

    print(f"\n{len(findings)} decode-breaking change(s):\n")
    for f in findings:
        print(f"  - {f}")
    print("\nA client generated from one of these releases cannot serve the other.")
    print("See the version policy in README.md.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
