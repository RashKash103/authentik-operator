#!/usr/bin/env python3
"""Render the Helm values table into docs/reference/helm-values.md.

helm-docs writes a chart README; this splices the values table out of it and
into the docs page, so the table cannot drift from values.yaml.

Usage:
    hack/gen-helm-docs.py [--check]
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CHART = ROOT / "charts" / "authentik-operator"
TARGET = ROOT / "docs" / "reference" / "helm-values.md"
HELM_DOCS = ROOT / "bin" / "helm-docs"
BEGIN = "<!-- BEGIN GENERATED: helm-values -->"
END = "<!-- END GENERATED: helm-values -->"


def values_table() -> str:
    """Run helm-docs and return just the generated values table."""
    if not HELM_DOCS.exists():
        print(f"ERROR: {HELM_DOCS} not found; run 'make helm-docs'", file=sys.stderr)
        sys.exit(1)

    # Render to stdout so the chart's own README is never rewritten as a side
    # effect of building the docs site.
    result = subprocess.run(
        [str(HELM_DOCS), "--chart-search-root", str(CHART), "--dry-run"],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        print(f"ERROR: helm-docs failed:\n{result.stderr}", file=sys.stderr)
        sys.exit(1)

    output = result.stdout
    match = re.search(r"(\|\s*Key\s*\|.*)", output, re.DOTALL)
    if match is None:
        print("ERROR: helm-docs produced no values table", file=sys.stderr)
        sys.exit(1)
    return match.group(1).strip()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    page = TARGET.read_text()
    if BEGIN not in page or END not in page:
        print(f"ERROR: {TARGET.name} is missing its generated markers", file=sys.stderr)
        return 1

    desired = f"{BEGIN}\n\n{values_table()}\n\n{END}"
    updated = re.sub(
        re.escape(BEGIN) + r".*?" + re.escape(END), lambda _: desired, page, flags=re.DOTALL
    )

    if updated == page:
        print(f"{TARGET.name} is up to date")
        return 0
    if args.check:
        print(f"ERROR: {TARGET.name} is out of date; run 'make docs-helm' and commit", file=sys.stderr)
        return 1

    TARGET.write_text(updated)
    print(f"updated {TARGET.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
