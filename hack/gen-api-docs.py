#!/usr/bin/env python3
"""Render the generated API reference into docs/reference/api.md.

crd-ref-docs emits a whole document; this splices its body between the
generated markers in the page so the page's own prose survives regeneration.

Usage:
    hack/gen-api-docs.py <rendered-file>
    hack/gen-api-docs.py <rendered-file> --check
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
TARGET = ROOT / "docs" / "reference" / "api.md"
BEGIN = "<!-- BEGIN GENERATED: api -->"
END = "<!-- END GENERATED: api -->"


def body_of(rendered: str) -> str:
    """Strip crd-ref-docs' own title, which would duplicate the page heading."""
    text = rendered.strip()
    text = re.sub(r"^#\s+API Reference\s*\n+", "", text)
    return text.strip()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rendered", help="file produced by crd-ref-docs")
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    rendered = pathlib.Path(args.rendered).read_text()
    page = TARGET.read_text()

    if BEGIN not in page or END not in page:
        print(f"ERROR: {TARGET.name} is missing its generated markers", file=sys.stderr)
        return 1

    desired = f"{BEGIN}\n\n{body_of(rendered)}\n\n{END}"
    updated = re.sub(
        re.escape(BEGIN) + r".*?" + re.escape(END), lambda _: desired, page, flags=re.DOTALL
    )

    if updated == page:
        print(f"{TARGET.name} is up to date")
        return 0
    if args.check:
        print(f"ERROR: {TARGET.name} is out of date; run 'just docs-api' and commit", file=sys.stderr)
        return 1

    TARGET.write_text(updated)
    print(f"updated {TARGET.name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
