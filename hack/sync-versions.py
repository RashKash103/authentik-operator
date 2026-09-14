#!/usr/bin/env python3
"""Keep the supported authentik version matrix consistent across the repo.

supported-versions.yaml is the single source of truth. The same facts also
appear in the README table and in the Go version gate, and without a check they
drift: someone bumps the CI matrix, the README keeps advertising a version
nobody tests, and the operator's runtime gate disagrees with both.

Usage:
    hack/sync-versions.py           rewrite derived files in place
    hack/sync-versions.py --check   exit non-zero if anything is out of date
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
SOURCE = ROOT / "supported-versions.yaml"
README = ROOT / "README.md"
VERSION_GO = ROOT / "internal" / "authentik" / "version.go"

BEGIN = "<!-- BEGIN SUPPORTED-VERSIONS -->"
END = "<!-- END SUPPORTED-VERSIONS -->"


def load() -> dict:
    with SOURCE.open() as fh:
        return yaml.safe_load(fh)


def render_table(data: dict) -> str:
    """Render the table with padded columns so the committed markdown stays readable."""
    headers = ("authentik series", "Tested image", "Status")
    rows = [(f"`{e['series']}`", f"`{e['image']}`", "Supported") for e in data["supported"]]

    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]
    line = lambda cells: "| " + " | ".join(c.ljust(w) for c, w in zip(cells, widths)) + " |"

    out = [line(headers), "| " + " | ".join("-" * w for w in widths) + " |"]
    out.extend(line(r) for r in rows)
    return "\n".join(out)


def sync_readme(data: dict, check: bool) -> bool:
    text = README.read_text()
    if BEGIN not in text or END not in text:
        print(f"ERROR: {README.name} is missing the {BEGIN} / {END} markers", file=sys.stderr)
        return False

    desired = f"{BEGIN}\n{render_table(data)}\n{END}"
    updated = re.sub(
        re.escape(BEGIN) + r".*?" + re.escape(END), desired, text, flags=re.DOTALL
    )

    if updated == text:
        return True
    if check:
        print(f"ERROR: {README.name} version table is out of date with {SOURCE.name}", file=sys.stderr)
        print("       run 'make sync-versions' and commit the result", file=sys.stderr)
        return False
    README.write_text(updated)
    print(f"updated {README.name}")
    return True


def check_go_constants(data: dict) -> bool:
    """The Go version gate must agree with the source of truth.

    This only checks; it never rewrites Go source, because the gate is hand
    written and a regex rewrite of it would be more dangerous than useful.
    """
    if not VERSION_GO.exists():
        print(f"note: {VERSION_GO.relative_to(ROOT)} does not exist yet, skipping gate check")
        return True

    text = VERSION_GO.read_text()
    ok = True
    for name, want in (("MinimumVersion", data["minimum"]), ("MaximumVersion", data["maximum"])):
        match = re.search(rf'{name}\s*=\s*"([^"]+)"', text)
        if match is None:
            print(f"ERROR: {VERSION_GO.name} does not define {name}", file=sys.stderr)
            ok = False
        elif match.group(1) != want:
            print(
                f"ERROR: {VERSION_GO.name} {name} = {match.group(1)!r}, "
                f"but {SOURCE.name} says {want!r}",
                file=sys.stderr,
            )
            ok = False
    return ok


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    data = load()
    ok = sync_readme(data, args.check)
    ok = check_go_constants(data) and ok

    if ok and args.check:
        print("supported versions are consistent")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
