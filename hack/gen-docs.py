#!/usr/bin/env python3
"""Regenerate the documentation pages that are derived from other files.

Several pages under docs/ restate facts that live somewhere else: the supported
authentik version matrix comes from supported-versions.yaml, and a couple of
pages mirror root-level markdown files that Zensical cannot link to because they
sit outside docs/. Without a check these drift, and drifted documentation is
worse than none: it is confidently wrong.

Each derived block is delimited by HTML comment markers, the same convention
hack/sync-versions.py uses for the README:

    <!-- BEGIN SUPPORTED-VERSIONS -->
    ...replaced wholesale...
    <!-- END SUPPORTED-VERSIONS -->

Prose outside the markers is hand written and preserved. A page missing its
markers is an error rather than something to silently rewrite.

Usage:
    hack/gen-docs.py           rewrite derived pages in place
    hack/gen-docs.py --check   exit non-zero if anything is out of date
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
SOURCE = ROOT / "supported-versions.yaml"

VERSIONS_PAGE = DOCS / "operations" / "supported-versions.md"


def marker(name: str) -> tuple[str, str]:
    return f"<!-- BEGIN {name} -->", f"<!-- END {name} -->"


def load() -> dict:
    with SOURCE.open() as fh:
        return yaml.safe_load(fh)


def render_table(data: dict) -> str:
    """Render the version table with padded columns, matching sync-versions.py.

    The padding is not cosmetic fussiness: an unpadded table produces a noisy
    diff every time the longest image tag changes length.
    """
    headers = ("authentik series", "Tested image", "Status")
    rows = [(f"`{e['series']}`", f"`{e['image']}`", "Supported") for e in data["supported"]]

    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]

    def line(cells: tuple[str, ...]) -> str:
        return "| " + " | ".join(c.ljust(w) for c, w in zip(cells, widths)) + " |"

    out = [line(headers), "| " + " | ".join("-" * w for w in widths) + " |"]
    out.extend(line(r) for r in rows)
    return "\n".join(out)


def render_bounds(data: dict) -> str:
    return (
        f"- **Minimum:** `{data['minimum']}` — an inclusive lower bound "
        "enforced by the runtime gate.\n"
        f"- **Maximum tested:** `{data['maximum']}` — newer versions still "
        "reconcile, with a warning."
    )


def replace_block(text: str, name: str, body: str, path: pathlib.Path) -> str | None:
    """Swap the contents of one marked block. Returns None if the markers are absent."""
    begin, end = marker(name)
    if begin not in text or end not in text:
        print(
            f"ERROR: {path.relative_to(ROOT)} is missing the {begin} / {end} markers",
            file=sys.stderr,
        )
        return None
    return re.sub(
        re.escape(begin) + r".*?" + re.escape(end),
        f"{begin}\n{body}\n{end}",
        text,
        flags=re.DOTALL,
    )


def sync_page(path: pathlib.Path, blocks: dict[str, str], check: bool) -> bool:
    """Apply every named block to one page. Returns False on error or drift."""
    if not path.exists():
        print(f"ERROR: {path.relative_to(ROOT)} does not exist", file=sys.stderr)
        return False

    original = path.read_text()
    updated = original
    for name, body in blocks.items():
        result = replace_block(updated, name, body, path)
        if result is None:
            return False
        updated = result

    rel = path.relative_to(ROOT)
    if updated == original:
        return True
    if check:
        print(f"ERROR: {rel} is out of date with {SOURCE.name}", file=sys.stderr)
        print("       run 'hack/gen-docs.py' and commit the result", file=sys.stderr)
        return False
    path.write_text(updated)
    print(f"updated {rel}")
    return True


def check_nav_pages() -> bool:
    """Every page named in zensical.toml's nav must exist, or the site build fails.

    Parsed with a regex rather than a TOML library: the nav is a nested mix of
    tables and arrays, and all this needs is the set of quoted *.md strings.
    Catching a missing page here gives a clearer error than the build does.
    """
    config = ROOT / "zensical.toml"
    if not config.exists():
        print("note: zensical.toml does not exist, skipping nav check")
        return True

    text = config.read_text()
    nav_match = re.search(r"^nav\s*=\s*\[", text, flags=re.MULTILINE)
    if nav_match is None:
        print("note: zensical.toml declares no nav, skipping nav check")
        return True

    # Take everything from `nav = [` to the first line that closes it at column 0.
    tail = text[nav_match.end():]
    close = re.search(r"^\]", tail, flags=re.MULTILINE)
    nav = tail[: close.start()] if close else tail

    ok = True
    for rel in re.findall(r'"([^"]+\.md)"', nav):
        if not (DOCS / rel).exists():
            print(f"ERROR: zensical.toml nav lists docs/{rel}, which does not exist", file=sys.stderr)
            ok = False
    return ok


def check_orphan_pages() -> bool:
    """Warn about pages on disk that no nav entry references.

    Not fatal: a page can legitimately be staged before its nav entry lands.
    """
    config = ROOT / "zensical.toml"
    if not config.exists():
        return True

    listed = set(re.findall(r'"([^"]+\.md)"', config.read_text()))
    for path in sorted(DOCS.rglob("*.md")):
        rel = path.relative_to(DOCS).as_posix()
        if rel not in listed:
            print(f"note: docs/{rel} is not listed in zensical.toml nav")
    return True


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    if not SOURCE.exists():
        print(f"ERROR: {SOURCE.name} not found", file=sys.stderr)
        return 1

    data = load()

    ok = sync_page(
        VERSIONS_PAGE,
        {
            "SUPPORTED-VERSIONS": render_table(data),
            "SUPPORTED-VERSIONS-BOUNDS": render_bounds(data),
        },
        args.check,
    )
    ok = check_nav_pages() and ok
    check_orphan_pages()

    if ok and args.check:
        print("derived documentation is up to date")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
