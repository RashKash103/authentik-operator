# Supported authentik versions

`supported-versions.yaml` at the repository root is the single source of truth
for which authentik releases this operator supports. It feeds the CI E2E matrix,
the table in `README.md`, the table below, and the runtime version gate.

authentik ships CalVer minor series (`YYYY.M`). **One operator release targets
one series**, pinned to an exact patch tag so CI is reproducible.

## Version policy

Each operator release is built against a specific authentik series and pinned to
the API client generated from it. When authentik ships a release that client
cannot serve, that is not a patch — it is a **new versioned release of the
operator and its Helm chart**, targeting the new authentik.

- **Track the latest.** The newest operator release always targets the newest
  supported authentik.
- **Pin to match your authentik.** Running an older authentik means staying on
  the operator release built for it. The image and chart are both versioned and
  immutable, so an older pairing keeps working; it just stops gaining features.
- **Upgrade in step.** Crossing a boundary means moving the operator with
  authentik.

| Operator / chart | authentik | Notes                                  |
| ---------------- | --------- | -------------------------------------- |
| `0.1.x`          | `2026.8`  | First release. API version `v1alpha1`. |

??? question "Why one version per release rather than a range?"

    `goauthentik.io/api/v3` is generated from a single authentik release and
    enforces *that* release's required properties when decoding. The pinned
    client marks `Application.pbm_uuid` and `SAMLProvider.url_issuer` as
    required, and neither exists in 2026.5 or 2026.2, so every list call against
    those versions fails outright.

    The E2E matrix found this rather than a user: 2026.8 passed while 2026.5 and
    2026.2 failed. Connections worked on all three; applications and SAML
    providers did not.

    Supporting a genuine range would mean pinning a client generated from the
    *oldest* version to support — a newer authentik returns a superset of
    fields, so an older client's required set is satisfied — or decoding
    tolerantly instead of through the generated models. Until one of those is
    done, pinning per release is the honest arrangement.

## The matrix

<!--
  GENERATED — do not edit the table between the markers by hand.
  Source: supported-versions.yaml. Regenerate with `hack/gen-docs.py`, or
  `just sync-versions`, which also updates the copy in README.md.
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
just sync-versions      # rewrite README.md's table
just verify-versions    # fail if anything is out of date
hack/gen-docs.py        # rewrite this page's table
hack/gen-docs.py --check
```

`just verify` runs the version check as part of its normal work.

!!! note "The Go gate is checked, never rewritten"

    `hack/sync-versions.py` verifies that `internal/authentik/version.go`
    declares `MinimumVersion` and `MaximumVersion` matching this file, and fails
    if they disagree. It does not edit Go source: the gate is hand-written, and
    a regex rewrite of it would be more dangerous than useful.


## See also

- [Conditions](../reference/conditions.md#unsupportedversion)
- [Troubleshooting](troubleshooting.md#unsupportedversion)
- [Connections](../guides/connections.md#the-version-gate)
