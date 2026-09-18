#!/usr/bin/env python3
"""Keep the chart version consistent everywhere the docs quote it.

charts/authentik-operator/Chart.yaml is the single source of truth. The same
version also appears in install commands across the README, the docs site, the
chart's own README and the Flux examples. Cutting a release by hand means
editing eight files, and the one that gets missed tells a reader to install a
version that does not exist.

Each site is rewritten by pattern rather than by marker: these are install
commands people copy, and wrapping them in HTML comments would be worse to read
than the drift is to fix.

Usage:
    hack/sync-release-version.py           rewrite derived files in place
    hack/sync-release-version.py --check   exit non-zero if anything is stale
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

import yaml

ROOT = pathlib.Path(__file__).resolve().parent.parent
CHART = ROOT / "charts" / "authentik-operator" / "Chart.yaml"

SEMVER = r"\d+\.\d+\.\d+"


def chart_version() -> str:
    with CHART.open() as fh:
        chart = yaml.safe_load(fh)

    version = str(chart["version"])
    app = str(chart["appVersion"])
    if version != app:
        print(
            f"ERROR: Chart.yaml version ({version}) and appVersion ({app}) differ.\n"
            "       This project releases the operator and the chart together, so "
            "they move as one.",
            file=sys.stderr,
        )
        sys.exit(1)
    if re.fullmatch(SEMVER, version) is None:
        print(f"ERROR: Chart.yaml version {version!r} is not a bare semver", file=sys.stderr)
        sys.exit(1)
    return version


def next_minor(version: str) -> str:
    """The exclusive upper bound a caret-style Flux range uses."""
    major, minor, _ = (int(p) for p in version.split("."))
    return f"{major}.{minor + 1}.0"


def rules(version: str) -> list[tuple[pathlib.Path, str, str]]:
    """(path, pattern, replacement) for every place the version is quoted."""
    upper = next_minor(version)
    chart_ref = "oci://ghcr.io/rashkash103/charts/authentik-operator"
    image = "ghcr.io/rashkash103/authentik-operator"

    return [
        # `helm install --version X` and `helm show ... --version X`
        (ROOT / "README.md", rf"--version {SEMVER}", f"--version {version}"),
        (ROOT / "docs" / "getting-started" / "installation.md",
         rf"--version {SEMVER}", f"--version {version}"),
        (ROOT / "charts" / "authentik-operator" / "README.md",
         rf"--version {SEMVER}", f"--version {version}"),

        # The chart README's shields.io badges.
        (ROOT / "charts" / "authentik-operator" / "README.md",
         rf"Version: {SEMVER}", f"Version: {version}"),
        (ROOT / "charts" / "authentik-operator" / "README.md",
         rf"Version-{SEMVER}-informational", f"Version-{version}-informational"),
        (ROOT / "charts" / "authentik-operator" / "README.md",
         rf"AppVersion: {SEMVER}", f"AppVersion: {version}"),
        (ROOT / "charts" / "authentik-operator" / "README.md",
         rf"AppVersion-{SEMVER}-informational", f"AppVersion-{version}-informational"),

        # `IMG=...:vX` just bundle
        (ROOT / "README.md", rf"{re.escape(image)}:v{SEMVER}", f"{image}:v{version}"),
        (ROOT / "docs" / "getting-started" / "installation.md",
         rf"authentik-operator:v{SEMVER}", f"authentik-operator:v{version}"),

        # The kustomize component is pinned to a git tag, so it moves too.
        (ROOT / "docs" / "guides" / "references.md",
         rf"\?ref=v{SEMVER}", f"?ref=v{version}"),
        (ROOT / "examples" / "property-mappings" / "README.md",
         rf"\?ref=v{SEMVER}", f"?ref=v{version}"),

        # Flux OCIRepository tags and the semver range beside them.
        (ROOT / "docs" / "guides" / "gitops-flux.md",
         rf'tag: "{SEMVER}"', f'tag: "{version}"'),
        (ROOT / "docs" / "guides" / "gitops-flux.md",
         rf'semver: ">={SEMVER} <{SEMVER}"', f'semver: ">={version} <{upper}"'),
        (ROOT / "examples" / "flux" / "README.md",
         rf'semver: ">={SEMVER} <{SEMVER}"', f'semver: ">={version} <{upper}"'),
        (ROOT / "examples" / "flux" / "operator" / "ocirepository.yaml",
         rf'tag: "{SEMVER}"', f'tag: "{version}"'),
    ]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="verify without writing")
    args = parser.parse_args()

    version = chart_version()
    stale: list[str] = []
    touched: set[pathlib.Path] = set()
    contents: dict[pathlib.Path, str] = {}

    for path, pattern, replacement in rules(version):
        if path not in contents:
            if not path.exists():
                print(f"ERROR: {path.relative_to(ROOT)} does not exist", file=sys.stderr)
                return 1
            contents[path] = path.read_text()

        updated, count = re.subn(pattern, replacement, contents[path])
        if count == 0:
            # A pattern that matches nothing means the file was restructured and
            # this script is now silently doing nothing for it.
            print(
                f"ERROR: {path.relative_to(ROOT)} has no match for {pattern!r}; "
                "the version reference moved or was removed",
                file=sys.stderr,
            )
            return 1
        if updated != contents[path]:
            touched.add(path)
        contents[path] = updated

    for path in sorted(touched):
        rel = path.relative_to(ROOT)
        if args.check:
            stale.append(str(rel))
            continue
        path.write_text(contents[path])
        print(f"updated {rel}")

    if stale:
        print(
            f"ERROR: these files do not quote chart version {version}: "
            + ", ".join(stale),
            file=sys.stderr,
        )
        print("       run 'hack/sync-release-version.py' and commit the result", file=sys.stderr)
        return 1

    if args.check:
        print(f"release version {version} is consistent")
    elif not touched:
        print(f"release version {version} is consistent")
    return 0


if __name__ == "__main__":
    sys.exit(main())
