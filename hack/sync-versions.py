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
COMPOSE = ROOT / "test" / "authentik" / "docker-compose.yaml"
COMPOSE_LIB = ROOT / "test" / "authentik" / "lib.sh"
RELEASES_URL = "https://github.com/RashKash103/authentik-operator/releases/tag"
VERSIONS_PAGE = ROOT / "docs" / "operations" / "supported-versions.md"
DOCS_INDEX = ROOT / "docs" / "index.md"
VERSION_GO = ROOT / "internal" / "authentik" / "version.go"

BEGIN = "<!-- BEGIN SUPPORTED-VERSIONS -->"
END = "<!-- END SUPPORTED-VERSIONS -->"
PREVIOUS_BEGIN = "<!-- BEGIN PREVIOUS-VERSIONS -->"
PREVIOUS_END = "<!-- END PREVIOUS-VERSIONS -->"
MATRIX_BEGIN = "<!-- BEGIN COMPATIBILITY-MATRIX -->"
MATRIX_END = "<!-- END COMPATIBILITY-MATRIX -->"
BOUNDS_BEGIN = "<!-- BEGIN SUPPORTED-VERSIONS-BOUNDS -->"
BOUNDS_END = "<!-- END SUPPORTED-VERSIONS-BOUNDS -->"


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


def render_matrix(data: dict) -> str:
    """Render the operator-to-authentik pairing table.

    Hand-maintained, this table still advertised only 0.1.x after 0.2.0 had
    shipped. Rendering it from the same file the rest of the version facts come
    from means a release that forgets it fails the check instead.
    """
    headers = ("Operator / chart", "authentik", "Notes")
    rows = [
        (f"[`{e['operator']}`]({RELEASES_URL}/{e['tag']})", f"`{e['authentik']}`", e["notes"])
        for e in data.get("releases", [])
    ]
    if not rows:
        return "| " + " | ".join(headers) + " |\n| --- | --- | --- |"

    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]

    def line(cells: tuple[str, ...]) -> str:
        return "| " + " | ".join(c.ljust(w) for c, w in zip(cells, widths)) + " |"

    out = [line(headers), "| " + " | ".join("-" * w for w in widths) + " |"]
    out.extend(line(r) for r in rows)
    return "\n".join(out)


def render_previous(data: dict) -> str:
    """Render the authentik-first lookup: "I run X, which operator do I need?"

    The compatibility matrix answers the question the other way round, which is
    the wrong direction for the person who has an authentik already and is
    trying to find an operator release that speaks to it. Same data, indexed by
    what they know.
    """
    by_series: dict[str, list[dict]] = {}
    for entry in data.get("releases", []):
        by_series.setdefault(str(entry["authentik"]), []).append(entry)

    supported = {str(e["series"]) for e in data["supported"]}

    def semver(tag: str) -> tuple[int, ...]:
        return tuple(int(part) for part in tag.lstrip("v").split("."))

    headers = ("authentik", "Use operator", "Status")
    rows = []
    # Newest authentik first: someone on the current release should not have to
    # read past the history to find their row. One release per series - the
    # newest that supports it - because the question is "which one do I install",
    # and listing every release that ever supported it answers a different one.
    for series in sorted(by_series, reverse=True):
        newest = max(by_series[series], key=lambda e: semver(str(e["tag"])))
        link = f"[{newest['tag']}]({RELEASES_URL}/{newest['tag']})"
        status = "Supported" if series in supported else "Superseded"
        rows.append((f"`{series}`", link, status))

    if not rows:
        return "| " + " | ".join(headers) + " |\n| --- | --- | --- |"

    widths = [max(len(h), *(len(r[i]) for r in rows)) for i, h in enumerate(headers)]

    def line(cells: tuple[str, ...]) -> str:
        return "| " + " | ".join(c.ljust(w) for c, w in zip(cells, widths)) + " |"

    out = [line(headers), "| " + " | ".join("-" * w for w in widths) + " |"]
    out.extend(line(r) for r in rows)
    return "\n".join(out)


def render_bounds(data: dict) -> str:
    """Render the prose bounds, so they cannot contradict the table above them."""
    return (
        f"- **Minimum:** `{data['minimum']}`. The operator refuses to reconcile against "
        "anything older, and says so in a condition.\n"
        f"- **Maximum tested:** `{data['maximum']}`. Newer versions still reconcile, but the "
        "operator logs a warning that it is running untested."
    )


def sync_page(path: pathlib.Path, data: dict, check: bool, blocks) -> bool:
    """Apply the version-derived blocks this page owns.

    Which blocks each page gets is listed explicitly rather than inferred from
    the markers present. The docs page carries the supported-versions table and
    bounds too, but hack/gen-docs.py renders those with docs-specific wording -
    and when both scripts claimed them they rewrote each other on every run,
    so `just verify` could never pass twice in a row.
    """
    text = path.read_text()

    updated = text
    for begin, end, body in blocks:
        if begin not in updated:
            continue
        desired = f"{begin}\n{body}\n{end}"
        updated = re.sub(
            re.escape(begin) + r".*?" + re.escape(end),
            lambda _, d=desired: d,
            updated,
            flags=re.DOTALL,
        )

    if updated == text:
        return True
    if check:
        print(f"ERROR: {path.name} version tables are out of date with {SOURCE.name}", file=sys.stderr)
        print("       run 'just sync-versions' and commit the result", file=sys.stderr)
        return False
    path.write_text(updated)
    print(f"updated {path.name}")
    return True


def sync_local_stack(data: dict, check: bool) -> bool:
    """Keep the local authentik stack on the tested image tag.

    The tag was written in two more places than anyone would look for: a shell
    default in lib.sh, which up.sh exports, and a compose interpolation default
    that only applies if someone runs `docker compose` directly. Neither was
    checked, and lib.sh wins at runtime - so after the source of truth moved to
    2026.8.3, CI tested 2026.8.3 while `just authentik-up` still started
    2026.8.2. Tests that disagree about what they are testing are worse than
    tests that fail.
    """
    tag = str(data["supported"][-1]["image"]).rsplit(":", 1)[-1]

    targets = (
        # (path, pattern, description) - lib.sh is the one that takes effect.
        (COMPOSE_LIB, re.compile(r'(: "\$\{AUTHENTIK_TAG:=)([^}]+)(\}")'), "shell default"),
        (COMPOSE, re.compile(r"(\$\{AUTHENTIK_TAG:-)([^}]+)(\})"), "compose default"),
    )

    ok = True
    for path, pattern, what in targets:
        text = path.read_text()
        match = pattern.search(text)
        if match is None:
            print(f"ERROR: {path.name} has no AUTHENTIK_TAG {what} to sync", file=sys.stderr)
            ok = False
            continue
        if match.group(2) == tag:
            continue
        if check:
            print(f"ERROR: {path.name} {what} is authentik {match.group(2)}, "
                  f"but the tested image is {tag}", file=sys.stderr)
            print("       run 'just sync-versions' and commit the result", file=sys.stderr)
            ok = False
            continue
        path.write_text(pattern.sub(rf"\g<1>{tag}\g<3>", text, count=1))
        print(f"updated {path.name}")
    return ok


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
    # The README owns every block. The docs page owns only the two gen-docs.py
    # does not render, so the two scripts never touch the same marker.
    readme_blocks = (
        (BEGIN, END, render_table(data)),
        (BOUNDS_BEGIN, BOUNDS_END, render_bounds(data)),
        (MATRIX_BEGIN, MATRIX_END, render_matrix(data)),
        (PREVIOUS_BEGIN, PREVIOUS_END, render_previous(data)),
    )
    docs_blocks = (
        (MATRIX_BEGIN, MATRIX_END, render_matrix(data)),
        (PREVIOUS_BEGIN, PREVIOUS_END, render_previous(data)),
    )

    # The homepage carries only the older-authentik lookup; its supported table
    # comes from gen-docs.py, which owns that block on every docs page.
    index_blocks = ((PREVIOUS_BEGIN, PREVIOUS_END, render_previous(data)),)

    ok = True
    for page, blocks in (
        (README, readme_blocks),
        (VERSIONS_PAGE, docs_blocks),
        (DOCS_INDEX, index_blocks),
    ):
        ok = sync_page(page, data, args.check, blocks) and ok
    ok = sync_local_stack(data, args.check) and ok
    ok = check_go_constants(data) and ok

    if ok and args.check:
        print("supported versions are consistent")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
