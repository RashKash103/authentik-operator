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
