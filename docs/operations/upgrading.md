# Upgrading

## Before anything else: CRDs

Helm installs `crds/` on first install and **never touches them again** — not on
`helm upgrade`, not on `helm uninstall`. Every release that changes a CRD needs
them applied by hand, before the new operator starts:

```sh
kubectl apply --server-side -f https://github.com/RashKash103/authentik-operator/releases/download/v0.2.0/install.yaml
```

or, for a chart-only install, from the chart you are about to upgrade to:

```sh
helm show crds oci://ghcr.io/rashkash103/charts/authentik-operator --version 0.2.0 \
  | kubectl apply --server-side -f -
```

An operator running against the previous CRDs logs `no matches for kind` for
anything new and silently drops fields it cannot see.

---

## v0.2.0 → v0.3.0

Outpost membership moved from the outpost to the provider, and
`Outpost.providerRefs` is gone.

### Why

An outpost listing its providers meant every new application edited two
manifests, and made the outpost a shared file every team had to touch. A
provider now names the outposts that should serve it.

### Move each reference to its provider

```yaml
# Before, on the Outpost
spec:
  type: proxy
  providerRefs:
    - kind: ProxyProvider
      name: tool-a
```

```yaml
# After, on the ProxyProvider
spec:
  outpostRefs:
    - name: edge
```

The `Outpost` keeps everything else; delete only its `providerRefs`. Manifests
that still carry the field are rejected, so nothing silently keeps the old
meaning.

!!! warning "Move them in one apply"

    A provider whose reference has been deleted from the outpost but not yet
    added to itself is attached to nothing, and whatever it protects is served
    without authentication until the next reconcile fixes it. Apply both
    changes together.

### Membership is now additive

The operator records what it attached in `status.managedProviderIDs` and
detaches only from that set. A provider attached by hand, or by a second
operator instance sharing the authentik, is left alone instead of being
silently removed on the next reconcile — which is what the previous
full-overwrite behaviour did.

An outpost adopted from v0.2.0 starts with an empty `managedProviderIDs`, so its
existing providers read as somebody else's and survive. Add the matching
`outpostRefs` and the operator takes ownership of them again.

### New: outposts the operator does not own

`Outpost` can now reference one instead of creating it — `embedded: true` for
authentik's built-in outpost, or `existingOutpostName` for one somebody else
set up. `type`, `config`, `serviceConnectionRef` and `name` are rejected there,
and `deletionPolicy` is forced to `Orphan`. See
[Outposts the operator does not own](../guides/outposts.md#outposts-the-operator-does-not-own).

### New: attach to a provider the operator did not create

`providerRef.existingProviderName` names a provider that already exists in
authentik. The field has been in the CRD since v0.1.0 and never worked; it does
now. If you tried it and got `resource name may not be empty`, that was this.

---

## v0.1.0 → v0.2.0

This release changes how a provider names the flows, property mappings and
certificate key pairs it uses. **Manifests written for v0.1.0 are rejected by
the v0.2.0 CRDs**, so plan on editing them rather than discovering it at apply
time.

!!! warning "Apply the new CRDs before upgrading the operator"

    Three kinds are new — `Flow`, `PropertyMapping` and `CertificateKeyPair` —
    and providers cannot reconcile without them.

### What changed, and why

A provider used to carry the slug or name inline:

```yaml
authorizationFlow: default-provider-authorization-implicit-consent
```

It now references a resource that identifies the authentik object:

```yaml
authorizationFlow:
  name: provider-authorization     # a Flow resource
```

The slug appeared in every provider that used that flow and nothing connected
them, so changing which flow your providers authorise against meant editing each
one and hoping none were missed. It also put the decision in the wrong place:
when the operator learns to *create* flows, that lands in the `Flow` manifest and
nothing referencing it changes.

### 1. Declare the objects your providers reference

One `Flow`, `PropertyMapping` or `CertificateKeyPair` per distinct authentik
object, naming what you previously wrote inline:

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Flow
metadata:
  name: provider-authorization
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingSlug: default-provider-authorization-implicit-consent   # the old value
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: CertificateKeyPair
metadata:
  name: self-signed
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingName: authentik Self-signed Certificate                 # the old value
```

`existingSlug` and `existingName` take exactly the string the provider used to
carry, so this step is mechanical.

### 2. Point the providers at them

| Field | v0.1.0 | v0.2.0 |
| --- | --- | --- |
| `authorizationFlow`, `invalidationFlow`, `authenticationFlow` | slug string | `{name: <Flow>}` |
| `propertyMappings` | list of name strings | list of `{name: <PropertyMapping>}` |
| `nameIDMapping`, `authnContextClassRefMapping` (SAML) | name string | `{name: <PropertyMapping>}` |
| `verificationKeyPair`, `encryptionKeyPair` (SAML) | name string | `{name: <CertificateKeyPair>}` |
| `certificate` (proxy) | name string | `{name: <CertificateKeyPair>}` |
| `tlsVerification`, `tlsAuthentication` (Docker) | name string | `{name: <CertificateKeyPair>}` |
| `serviceConnectionRef` (outpost) | name string | `{kubernetesServiceConnectionName: …}` or `{dockerServiceConnectionName: …}` |

Two fields were also renamed on `OAuth2Provider`, to match the SAML provider and
to say what they actually hold:

| v0.1.0 | v0.2.0 |
| --- | --- |
| `signingKey` | `signingKeyPair` |
| `encryptionKey` | `encryptionKeyPair` |

### 3. Nothing in authentik is touched

References resolve to the same authentik objects, so the operator sees no drift
and rewrites nothing. `status.remoteID` is unchanged, providers stay adopted,
and no credential is regenerated.

!!! tip "Apply everything at once"

    A provider applied before its `Flow` reports `Ready=False` with reason
    `ReferenceNotFound` and converges as soon as the `Flow` resolves. There is no
    ordering to get right — `kubectl apply -f .` settles.

### New in this release, and optional

Neither of these changes behaviour unless you opt in.

- **`connection.spec.cluster`** — set it when several operators share one
  authentik, so each manages only its own objects. It becomes part of an
  object's identity, so setting it on an existing connection orphans what that
  operator already created. See
  [Several operators, one authentik](../guides/connections.md#several-operators-one-authentik).
- **`--allow-cross-namespace-references`** (chart value
  `allowCrossNamespaceReferences`) — lets a reference name another namespace, so
  one team can publish shared `Flow` resources. See
  [Namespaces](../guides/references.md#namespaces).

### Rolling back

v0.1.0 manifests are not valid against v0.2.0 CRDs and the reverse is also true,
so a rollback means restoring both the old CRDs and the old manifests. The
authentik objects themselves are untouched either way.
