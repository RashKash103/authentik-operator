# Shared property mappings

Almost every OIDC provider attaches the same three scope mappings — `openid`,
`email`, `profile` — and declaring them by hand in every namespace is the sort
of boilerplate that invites copy-paste drift.

`oidc/` is a [kustomize
component](https://kubectl.docs.kubernetes.io/guides/config_management/components/)
that ships them. Include it from any kustomization:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: my-apps

components:
  - github.com/RashKash103/authentik-operator/examples/property-mappings/oidc?ref=v0.2.0
```

Providers then reference them by resource name:

```yaml
propertyMappings:
  - name: oidc-openid
  - name: oidc-email
  - name: oidc-profile
```

`example-usage/` is a working kustomization you can build to see the result:

```sh
kustomize build examples/property-mappings/example-usage
```

## Pointing at a differently named connection

The component names a connection called `primary`. Patch it in the consuming
kustomization rather than editing the component, so one copy serves namespaces
that name their connections differently:

```yaml
patches:
  - target:
      kind: PropertyMapping
    patch: |
      - op: replace
        path: /spec/connectionRef/name
        value: default
```

## Why this is not built into the operator

The operator could attach these three whenever `propertyMappings` is empty. It
deliberately does not, for three reasons.

**authentik replaces the list, it does not merge it.** If defaults appeared when
the field was empty, then setting one custom mapping would silently drop them —
the field would mean "these three plus yours" when empty and "only yours" when
not. That is learned by having tokens quietly lose claims in production.

**The names are authentik's, not this operator's.** They read
`authentik default OAuth Mapping: OpenID 'email'`, colon and quotes included,
and authentik is free to rename them. Compiling them into the operator couples
a release to strings it does not own, which is the same coupling that already
pins the operator to one authentik series.

**A token's claims are a security surface.** What an application receives should
be readable from its manifest, not from knowing what the operator adds when you
leave a field out.

A kustomize component gets the boilerplate out of your manifests while keeping
the result explicit, greppable, and overridable without a merge-versus-replace
trap.

## Other defaults worth knowing

The same pattern works for anything else you attach repeatedly. authentik also
ships, among others:

| Mapping | Use |
| --- | --- |
| `authentik default OAuth Mapping: OpenID 'offline_access'` | Refresh tokens for clients that ask for `offline_access`. |
| `authentik default OAuth Mapping: authentik API access` | Tokens that may call the authentik API itself. Attach deliberately. |
| `authentik default SAML Mapping: Username` | SAML providers, alongside the other `SAML Mapping` entries. |

Confirm what your authentik has with:

```sh
curl -sH "Authorization: Bearer $TOKEN" \
  "$AUTHENTIK_URL/api/v3/propertymappings/all/?page_size=200" \
  | jq -r '.results[].name'
```
