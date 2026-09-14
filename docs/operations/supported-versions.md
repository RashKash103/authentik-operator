# Supported authentik versions

`supported-versions.yaml` at the repository root is the single source of truth
for which authentik releases this operator supports. It feeds the CI E2E matrix,
the table in `README.md`, the table below, and the runtime version gate.

authentik ships CalVer minor series (`YYYY.M`). This project supports the
**three most recent series** and pins an exact patch tag per series so CI is
reproducible.

## The matrix

<!--
  GENERATED — do not edit the table between the markers by hand.
  Source: supported-versions.yaml. Regenerate with `hack/gen-docs.py`, or
  `make sync-versions`, which also updates the copy in README.md.
  `hack/gen-docs.py --check` fails if this drifts.
-->

<!-- BEGIN SUPPORTED-VERSIONS -->
| authentik series | Tested image                          | Status    |
| ---------------- | ------------------------------------- | --------- |
| `2026.8`         | `ghcr.io/goauthentik/server:2026.8.2` | Supported |
<!-- END SUPPORTED-VERSIONS -->

<!-- BEGIN SUPPORTED-VERSIONS-BOUNDS -->
- **Minimum:** `2026.8` — an inclusive lower bound enforced by the runtime gate.
- **Maximum tested:** `2026.8` — newer versions still reconcile, with a warning.
<!-- END SUPPORTED-VERSIONS-BOUNDS -->

The pinned patch tags are what CI runs against. Any patch within a supported
series is expected to work; only the pinned one is actually tested.

## How the gate behaves

A connection probes its instance and records what it finds:

```sh
$ kubectl -n my-apps get authentikconnection default -o wide
NAME      URL                             VERSION    READY   AGE
default   https://authentik.example.com   2026.8.2   True    4h
```

| Detected version | `status.versionSupported` | Behaviour |
| --- | --- | --- |
| Below the minimum | `false` | Dependent resources **refuse to reconcile** and report `UnsupportedVersion`. |
| Within the range | `true` | Normal operation. |
| Above the maximum tested | `true` | Reconciles normally; the operator logs a warning that it is running untested. |

!!! success "Failing early is the point"

    The alternative to a version gate is failing obscurely deep inside an API
    call against a schema that changed — a 400 carrying an authentik-internal
    message, three layers away from the manifest that caused it.

    Refusing up front and naming the version is far cheaper to diagnose.

!!! note "Above the maximum is a warning, not an error"

    Being *ahead* of the test matrix is a risk you can knowingly accept. Being
    *behind* the minimum means calls that are known to fail, so that one is a
    hard stop.

## Checking your version

=== "From the cluster"

    ```sh
    kubectl -n my-apps get authentikconnection default \
      -o jsonpath='{.status.authentikVersion} supported={.status.versionSupported}{"\n"}'
    ```

=== "From authentik directly"

    ```sh
    curl -sS https://authentik.example.com/api/v3/root/config/ | jq -r .capabilities
    ```

    The version is also shown in the authentik admin interface under
    **System → Overview**.

## Upgrading

!!! warning "Upgrade authentik before dropping a series"

    When a series leaves this table, an operator release that no longer supports
    it will set `UnsupportedVersion` on every connection pointing at it, and
    every provider and application below will stop reconciling. Existing
    authentik objects are untouched — nothing is deleted — but no change you
    make takes effect.

    Upgrade authentik first, confirm `status.versionSupported` is `true`, then
    upgrade the operator.

Rough order for a combined upgrade:

1. Check the current versions of both.
2. Upgrade authentik to a series in this table.
3. Confirm every connection reports `Ready=True` and `versionSupported: true`.
4. [Apply the new CRDs](../getting-started/installation.md#upgrading-the-chart-crds-are-your-job)
   — Helm will not.
5. Upgrade the operator.

## Changing the supported set

`supported-versions.yaml` is the only file to edit. Everything else derives from
it, and CI fails if the derived copies drift:

```sh
make sync-versions      # rewrite README.md's table
make verify-versions    # fail if anything is out of date
hack/gen-docs.py        # rewrite this page's table
hack/gen-docs.py --check
```

`make verify` runs the version check as part of its normal work.

!!! note "The Go gate is checked, never rewritten"

    `hack/sync-versions.py` verifies that `internal/authentik/version.go`
    declares `MinimumVersion` and `MaximumVersion` matching this file, and fails
    if they disagree. It does not edit Go source: the gate is hand-written, and
    a regex rewrite of it would be more dangerous than useful.

    That file does not exist yet, and the check skips with a note until it does.

## See also

- [Conditions](../reference/conditions.md#unsupportedversion)
- [Troubleshooting](troubleshooting.md#unsupportedversion)
- [Connections](../guides/connections.md#the-version-gate)
