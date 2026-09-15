# Conditions

Every resource this operator manages reports its state through standard
Kubernetes conditions. There are exactly **two condition types** and **nine
reasons**, drawn from a fixed set of Go constants in
`api/v1alpha1/common_types.go` rather than invented per call site — which is
what makes this page possible to enumerate at all.

!!! note "This page is hand-written"

    The constants are generated-adjacent; the *cause and fix* for each is
    judgement. A reason added to `common_types.go` needs a row added here in the
    same pull request. See [Reference](index.md).

## Reading conditions

```sh
kubectl -n my-apps describe authentikconnection default
kubectl -n my-apps get authentikconnection default -o jsonpath='{.status.conditions}' | jq
```

Or wait on one:

```sh
kubectl -n my-apps wait --for=condition=Ready authentikconnection/default --timeout=60s
```

!!! tip "`observedGeneration` protects `kubectl wait`"

    Each condition carries the `metadata.generation` it was computed from. A
    `wait --for=condition=Ready` therefore cannot be satisfied by a stale
    `Ready=True` left over from the previous spec — a real hazard when you apply
    a change and immediately wait on it.

    If `status.observedGeneration` is behind `metadata.generation`, the operator
    has not processed your latest edit yet.

## Condition types

### `Ready`

**The resource matches its desired spec.** This is the one to gate on.

| Status | Meaning |
| --- | --- |
| `True` | The authentik object exists and matches the spec. Reason is always `Succeeded`. |
| `False` | It does not. The reason says why. |
| Absent | Not reconciled yet — or the namespace is not watched. |

### `Synced`

**The last reconcile reached the authentik API.** Deliberately separate from
`Ready`.

| Status | Meaning |
| --- | --- |
| `True` | The API was reached and responded. |
| `False` | It was not reachable or not usable. |

!!! tip "The two together tell you where the problem is"

    | `Synced` | `Ready` | Interpretation |
    | --- | --- | --- |
    | `True` | `True` | Healthy. |
    | `True` | `False` | authentik is reachable; **your spec or its references are the problem**. |
    | `False` | `False` | authentik is unreachable or rejecting authentication. Nothing about your spec has been evaluated. |
    | `False` | `True` | Should not occur. A sync failure always clears `Ready`. |

    Conflating them would hide which of the two actually failed, which is why
    `MarkNotReady` leaves `Synced` alone.

## Reasons

Every reason below, what causes it, and what to do.

### `Succeeded`

**Not an error.**

The resource reconciled cleanly. Set on both `Ready=True` and `Synced=True`,
with messages `Resource is in sync with authentik` and
`Last reconcile reached the authentik API`.

No action needed.

---

### `ReferenceNotFound`

**A referenced object does not exist — yet.**

| Referenced thing | Where it is looked for |
| --- | --- |
| Token or CA `Secret` | The connection's own namespace, or `tokenSecretRef.namespace` for the cluster kind |
| Connection | The referring resource's own namespace, always |
| Provider (from an `Application` or `Outpost`) | The referring resource's own namespace, or `namespace` where cross-namespace references are enabled |
| Flow | The `Flow` resource's `status.remoteID` |
| Property mapping | The `PropertyMapping` resource's `status.remoteID` |
| Certificate keypair | The `CertificateKeyPair` resource's `status.remoteID` |

**Common causes**

- A typo in a name, slug or key.
- The `Secret` key is wrong — the `Secret` exists, `key: token` does not.
- A `Flow`, `PropertyMapping` or `CertificateKeyPair` resource that has not
  resolved yet, or whose `existingSlug` names something that only exists in
  staging.
- **Ordering.** An `Application` applied before its provider. This is expected
  and self-healing; see below.
- A reference naming another namespace while the operator runs without
  `--allow-cross-namespace-references`. It is refused rather than resolved in
  the local namespace, which would silently pick a different object.
- A reference to a resource wired to a *different* authentik instance. The UUID
  it resolved to means nothing on the referring resource's instance.

**Fix**

```sh
kubectl -n my-apps describe application grafana   # the message names what was missing
kubectl -n my-apps get secret authentik-api-token -o jsonpath='{.data}' | jq 'keys'
```

Create the missing object. The resource requeues with backoff and converges on
its own — you do not need to re-apply anything.

!!! success "This is not a failure during a bulk apply"

    `Application.provider` is a numeric primary key that exists only after the
    provider is created, so an `Application` applied first *must* wait. It
    reports `ReferenceNotFound` for a few seconds and then converges.

    Seeing it for **minutes** means the provider itself is not becoming ready.
    Investigate the provider, not the application.

---

### `ReferenceAmbiguous`

**A reference matched more than one object in authentik.**

Property mapping names are not unique in authentik, so two mappings can
legitimately share one. The operator refuses to guess.

!!! danger "Why this is a hard failure rather than a first-match"

    Picking one would bind a mapping the author did not mean. The symptom is a
    token quietly missing a claim, discovered weeks later by an application
    behaving subtly wrong. There is no safe guess.

**Fix**

The condition message names the candidates. Rename one of them in authentik so
the reference is unambiguous, then let the resource requeue.

---

### `ConnectionNotReady`

**The referenced connection is unusable.**

The resource itself may be perfectly valid. Nothing was written to authentik.

**Common causes**

- The connection is not `Ready` — bad URL, bad token, unreachable instance.
- The connection's `status.versionSupported` is `false`.
- The referring namespace is not in the cluster connection's
  `allowedNamespaces`.
- `connectionRef.kind` is wrong — pointing at `AuthentikConnection` when you
  created a `ClusterAuthentikConnection`.

**Fix**

Always go one layer down first:

```sh
kubectl -n my-apps describe authentikconnection default
kubectl describe clusterauthentikconnection shared
```

Fix the connection. Everything referencing it recovers without intervention.

---

### `AdoptionConflict`

**A pre-existing authentik object with the same name or slug blocks creation.**

The operator found an object it cannot prove it created, and
`spec.adoptionPolicy` is `FailOnConflict` (the default).

!!! success "Nothing was modified"

    The reconcile stopped before writing. Whatever exists in authentik is
    untouched. This is the entire point of the default — see
    [ADR 0002](../decisions/0002-adoption-policy.md).

**Common causes**

- Someone created the object by hand, months ago, and has been tuning it since.
- Another cluster, or another operator installation, manages the same authentik.
- The Kubernetes resource was deleted with `deletionPolicy: Orphan` and
  recreated.
- Two namespaces declaring the same `slug`. authentik slugs are global;
  Kubernetes names are not.

**Fix** — decide which you mean:

=== "Take it over"

    ```yaml
    spec:
      adoptionPolicy: AdoptExisting
    ```

    The operator stamps its provenance marker and reconciles the object to
    spec. **Fields the operator manages are overwritten.**

    !!! warning

        Do not template this across everything. It is per-object precisely so
        importing one legacy provider does not lower the bar for the rest.

=== "Use a different name"

    Change `spec.slug` (applications) or the object name so it no longer
    collides. Nothing is overwritten.

=== "Remove the conflict"

    Delete the pre-existing object in authentik. The next reconcile creates a
    fresh one — at the cost of an outage for whatever used it.

!!! danger "A foreign provenance marker fails even under `AdoptExisting`"

    If the object carries a marker naming a *different* operator installation,
    adoption is refused regardless of policy, and the message names the other
    installation. Two operators overwriting each other in a loop is worse than a
    clear error.

---

### `UnsupportedVersion`

**The authentik version is outside the tested range.**

Checked against the connection's `status.authentikVersion`. Dependent resources
refuse to reconcile rather than failing obscurely inside an API call against a
changed schema.

**Fix**

```sh
kubectl -n my-apps get authentikconnection default -o jsonpath='{.status.authentikVersion}'
```

Compare against [Supported versions](../operations/supported-versions.md), then
either upgrade authentik into range, or use an operator release that supports
your version.

!!! note "Above the maximum still reconciles"

    A version newer than the tested maximum logs a warning and proceeds —
    `UnsupportedVersion` is set only below the **minimum**. Being ahead of the
    test matrix is a risk you can accept; being behind it means calls that are
    known to fail.

---

### `InvalidSpec`

**authentik rejected the spec.**

The request reached authentik and was refused — so this is a `Ready=False` with
`Synced=True`. The operator relays authentik's own validation message.

**Common causes**

- A malformed redirect URI, or a `regex` matching mode with an invalid pattern.
- A flow of the wrong designation — an authorization flow where an invalidation
  flow was expected.
- A field combination authentik forbids, e.g. a signing key on a provider type
  that has none.
- A duplicate slug that passed the adoption check but collides on write.
- `providerRefs` on an `Outpost` mixing kinds, or not matching `spec.type`.

**Fix**

Read the message — it is authentik's, not the operator's, so it names the field
authentik objects to.

```sh
kubectl -n my-apps describe oauth2provider grafana
```

!!! note "Very long messages are truncated"

    Condition messages are capped at the API server's 32768-byte limit and
    suffixed with `(truncated)`. An over-long message would make the whole
    status update fail, hiding the very error being reported. For the untruncated
    text, check the operator logs.

---

### `APIError`

**A transient or unexpected API failure.** The catch-all.

Sets `Synced=False` and `Ready=False` — the API was not reached, or not usably.

| Message mentions | Cause | Fix |
| --- | --- | --- |
| `401` / `403` | Bad, expired or wrong-intent token; **or a trailing newline in the `Secret` value** | Recreate the token with intent **API Token**; write it with `printf %s` and `--from-file`. |
| `connection refused`, `timeout`, `no such host` | Wrong URL, DNS the cluster cannot resolve, or a `NetworkPolicy` blocking egress | Check `spec.url`; check egress rules. |
| `certificate signed by unknown authority` | Private CA | Set `caBundleSecretRef`. Avoid `insecureSkipTLSVerify`. |
| `404` | `spec.url` includes `/api/v3`, or a trailing path | The operator appends the suffix itself. Use the bare base URL. |
| `500`, `502`, `503` | authentik is unhealthy or restarting | Usually resolves on its own; the resource requeues with backoff. |

!!! tip "Persistent `APIError` is usually credentials or DNS"

    A genuinely transient error clears within a couple of backoff cycles. One
    that persists for minutes is almost always the token, the URL, or a network
    policy — in that order of likelihood.

---

### `Deleting`

**Not an error.**

The resource is being torn down. Its finalizer is running: the operator is
deleting the authentik object (`deletionPolicy: Delete`) or releasing it
(`Orphan`), then removing the finalizer.

**If it is stuck here**, the finalizer cannot complete — almost always because
authentik is unreachable, so the operator cannot confirm the delete.

```sh
kubectl -n my-apps describe oauth2provider grafana   # look at the connection
```

Fix the connection and deletion completes on its own.

!!! danger "Removing a finalizer by hand orphans the authentik object"

    ```sh
    # Last resort only.
    kubectl -n my-apps patch oauth2provider grafana \
      -p '{"metadata":{"finalizers":[]}}' --type=merge
    ```

    The Kubernetes resource disappears; the authentik object stays, now managed
    by nobody, and will cause `AdoptionConflict` the next time something claims
    that name. Clean it up in authentik yourself.

## Summary

| Reason | Types affected | Error? | Self-heals? | First thing to check |
| --- | --- | --- | --- | --- |
| `Succeeded` | `Ready`, `Synced` | No | — | — |
| `ReferenceNotFound` | `Ready` | Sometimes | Yes | Spelling; whether the referenced resource exists yet |
| `ReferenceAmbiguous` | `Ready` | Yes | No | Duplicate names in authentik |
| `ConnectionNotReady` | `Ready` | Yes | Yes, once the connection recovers | The connection, one layer down |
| `AdoptionConflict` | `Ready` | Yes | No | What already exists in authentik under that name |
| `UnsupportedVersion` | `Ready` | Yes | Yes, after an upgrade | `status.authentikVersion` |
| `InvalidSpec` | `Ready` | Yes | No | The message — it is authentik's |
| `APIError` | `Ready`, `Synced` | Yes | Often | Token, then URL, then egress |
| `Deleting` | `Ready` | No | Yes | Only matters if stuck |

## See also

- [Troubleshooting](../operations/troubleshooting.md) — symptom-first, keyed to these reasons.
- [Supported versions](../operations/supported-versions.md)
- [ADR 0002 — Adoption policy](../decisions/0002-adoption-policy.md)
