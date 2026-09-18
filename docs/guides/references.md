# References between resources

Providers, applications and outposts do not name authentik objects inline. They
reference other resources, and those resources say which authentik object is
meant.

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Flow
metadata:
  name: provider-authorization
spec:
  connectionRef:
    name: primary
  existingSlug: default-provider-authorization-explicit-consent
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Flow
metadata:
  name: provider-invalidation
spec:
  connectionRef:
    name: primary
  existingSlug: default-provider-invalidation-flow
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: OAuth2Provider
metadata:
  name: grafana
spec:
  connectionRef:
    name: primary
  authorizationFlow:
    name: provider-authorization   # the Flow above
  invalidationFlow:
    name: provider-invalidation
```

## Why not just write the slug?

Because the slug would then appear in every provider that uses that flow, and
nothing would connect them. Swapping which flow your providers authorise
against would mean editing each one and hoping none were missed.

There is a second reason, and it is the one that matters more over time. These
resources currently only *adopt* an object that already exists in authentik.
When the operator learns to **create** flows, property mappings and key pairs,
that capability lands in the `Flow` manifest — and nothing referencing it
changes. Had the slug lived at the reference site, every provider would have
had to change instead.

## The reference kinds

| Resource | Identifies | Referenced by |
| --- | --- | --- |
| `Flow` | An authentik flow | `authorizationFlow`, `invalidationFlow`, `authenticationFlow` |
| `PropertyMapping` | A property mapping | `propertyMappings`, `nameIDMapping`, `authnContextClassRefMapping` |
| `CertificateKeyPair` | A certificate-key pair | `signingKeyPair`, `encryptionKeyPair`, `verificationKeyPair`, `certificate`, `tlsVerification`, `tlsAuthentication` |
| `OAuth2Provider`, `SAMLProvider`, `ProxyProvider` | A provider | `providerRef`, `backchannelProviderRefs` |
| `Outpost` | An outpost | `outpostRefs` on a provider |
| `KubernetesServiceConnection`, `DockerServiceConnection` | A service connection | `serviceConnectionRef` |

All of them resolve through the referenced resource's `status.remoteID`, never
by looking the name up in authentik. That keeps the Kubernetes object the source
of truth: renaming something inside authentik cannot silently repoint a provider
at a different object.

## Sharing the boilerplate ones

Almost every OIDC provider attaches the same three scope mappings, and declaring
them by hand in every namespace invites copy-paste drift. A kustomize
[component](https://kubectl.docs.kubernetes.io/guides/config_management/components/)
ships them:

```yaml title="kustomization.yaml"
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: my-apps

components:
  - github.com/RashKash103/authentik-operator/examples/property-mappings/oidc?ref=v0.2.0
```

That creates `oidc-openid`, `oidc-email` and `oidc-profile` in the namespace,
each adopting one of authentik's shipped defaults. Providers reference them by
resource name as usual:

```yaml
propertyMappings:
  - name: oidc-openid
  - name: oidc-email
  - name: oidc-profile
```

Or paste the three resources directly, if you would rather not take a dependency
on this repository:

```yaml title="oidc-mappings.yaml"
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: PropertyMapping
metadata:
  name: oidc-openid
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingName: "authentik default OAuth Mapping: OpenID 'openid'"
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: PropertyMapping
metadata:
  name: oidc-email
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingName: "authentik default OAuth Mapping: OpenID 'email'"
---
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: PropertyMapping
metadata:
  name: oidc-profile
  namespace: my-apps
spec:
  connectionRef:
    name: default
  existingName: "authentik default OAuth Mapping: OpenID 'profile'"
```

Quote the names: they contain a colon and single quotes, and YAML reads an
unquoted one as a mapping key.

!!! question "Why does the operator not just attach these by default?"

    Because authentik **replaces** a provider's mapping list rather than merging
    it. If defaults appeared whenever `propertyMappings` was empty, then adding
    one custom mapping would silently drop them, and the field would mean two
    different things depending on whether it was set. The failure shows up as an
    application missing a claim, weeks later.

    The names are also authentik's to rename, and what claims a token carries is
    a security surface that should be readable from the manifest rather than
    from knowing what the operator adds when you leave a field out.

    More in
    [`examples/property-mappings`](https://github.com/RashKash103/authentik-operator/tree/main/examples/property-mappings).

## Ordering does not matter

A provider applied before its `Flow` reports `Ready=False` with reason
`ReferenceNotFound`, naming the exact field and resource:

```
spec.authorizationFlow: Flow "provider-authorization" has not resolved in authentik yet
```

and converges as soon as the Flow reports an ID. Apply a whole directory in any
order; it settles.

## Namespaces

A reference with no `namespace` resolves in the referring resource's own
namespace. That is the default and, unless the operator has been told otherwise,
the only thing that works — two namespaces needing the same flow each declare a
`Flow`. They are cheap: a name and a slug.

Naming a namespace makes it a **cross-namespace reference**, which the operator
refuses unless it was started with `--allow-cross-namespace-references` (chart
value `allowCrossNamespaceReferences`):

```yaml
authorizationFlow:
  name: provider-authorization
  namespace: platform          # refused unless the operator allows it
```

The refusal is an error, not a quiet fall back to the local namespace: falling
back would resolve a *different object* than the manifest names.

It is off by default because it makes one namespace's configuration another's
dependency — the platform team can now break an application namespace by editing
a `Flow`. That is a reasonable thing to want, and a reasonable thing to want to
forbid, so it is a cluster-level decision rather than one a manifest author
makes for themselves.

Two limits apply even when it is enabled:

- **`connectionRef` never crosses a namespace.** Credentials are not
  configuration. Sharing a connection is the
  [`ClusterAuthentikConnection`](connections.md#clusterauthentikconnection)'s
  job, where the connection's *owner* grants access through `allowedNamespaces`
  rather than the consumer helping itself.
- **Both sides must talk to the same authentik.** A `Flow` resolves its slug to
  a UUID against one specific instance, and that UUID means nothing on another.
  A reference that crosses instances is rejected with a message saying so.

See [Several operators, one
authentik](connections.md#several-operators-one-authentik) for the related case:
several operators sharing one instance.
