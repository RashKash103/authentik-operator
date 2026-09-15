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
kind: OAuth2Provider
metadata:
  name: grafana
spec:
  connectionRef:
    name: primary
  authorizationFlow:
    name: provider-authorization   # the Flow above
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
| `OAuth2Provider`, `SAMLProvider`, `ProxyProvider` | A provider | `providerRef`, `providerRefs` |
| `KubernetesServiceConnection`, `DockerServiceConnection` | A service connection | `serviceConnectionRef` |

All of them resolve through the referenced resource's `status.remoteID`, never
by looking the name up in authentik. That keeps the Kubernetes object the source
of truth: renaming something inside authentik cannot silently repoint a provider
at a different object.

## Ordering does not matter

A provider applied before its `Flow` reports `Ready=False` with reason
`ReferenceNotFound`, naming the exact field and resource:

```
spec.authorizationFlow: Flow "provider-authorization" has not resolved in authentik yet
```

and converges as soon as the Flow reports an ID. Apply a whole directory in any
order; it settles.

## Namespaces

References resolve in the referring resource's own namespace, with no way to
express a cross-namespace reference. That is deliberate and matches every other
reference in this API: a cross-namespace reference would let anyone who can
create a provider in one namespace borrow configuration — and through it,
credentials — from another.

If two namespaces need the same flow, declare a `Flow` in each. They are cheap:
each one is a name and a slug.
