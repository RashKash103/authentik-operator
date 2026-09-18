# Providers

A provider is *how* authentication happens: an OAuth2/OIDC client, a SAML
service provider, or a proxy provider fronted by an outpost. An
[Application](applications.md) points at exactly one.

## The three kinds

| Kind | For | Needs an outpost |
| --- | --- | --- |
| `OAuth2Provider` | Apps speaking OAuth2 or OIDC | No |
| `SAMLProvider` | Apps speaking SAML 2.0 | No |
| `ProxyProvider` | Apps with no SSO support at all, fronted by forward-auth | **Yes** |

All three share the same envelope:

```yaml
spec:
  connectionRef:
    kind: AuthentikConnection      # default
    name: default
  adoptionPolicy: FailOnConflict   # default
  deletionPolicy: Delete           # default
```

## References point at other resources

authentik identifies flows, property mappings and certificate key pairs by
**UUID**, generated per instance. A manifest carrying one is not portable
between your staging and production authentik, tells a reviewer nothing, and
breaks silently when the object is recreated under a new UUID.

So a provider does not name any of them directly. It references a `Flow`,
`PropertyMapping` or `CertificateKeyPair` **resource**, and that resource says
which authentik object is meant:

```yaml
authorizationFlow:
  name: provider-authorization    # a Flow resource in this namespace
```

Resolution reads that resource's `status.remoteID` — it never looks the name up
in authentik. Kubernetes stays the source of truth, so renaming something inside
authentik cannot silently repoint a provider at a different object. See
[References between resources](references.md) for the full model, including
sharing one `Flow` across namespaces.

### Resolution failures

Every reference resolves before anything is written, so a failure never leaves a
half-configured provider in authentik.

`ReferenceNotFound`

:   The referenced resource does not exist, or exists but has not resolved its
    own authentik object yet. The second is ordinary right after a bulk apply
    and clears itself; the first is a typo. The condition message names the
    field and the resource.

`ReferenceAmbiguous`

:   The referenced resource matched more than one authentik object. **Property
    mapping names are not unique in authentik**, so this is a real possibility.
    It surfaces on the `PropertyMapping`, not on the provider.

!!! danger "Why ambiguity is a hard failure"

    Picking the first match would bind a scope mapping the author did not mean,
    and the symptom would be a token missing a claim — noticed weeks later, by
    an application behaving subtly wrong. There is no safe guess, so the
    operator stops and names both candidates in the condition message.

    Fix it by renaming one of the mappings in authentik so the reference is
    unambiguous.

## `OAuth2Provider`

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: OAuth2Provider
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    name: default

  authorizationFlow:
    name: provider-authorization
  invalidationFlow:
    name: provider-invalidation

  clientType: confidential          # or public
  redirectURIs:
    - matchingMode: strict
      url: https://grafana.example.com/login/generic_oauth

  propertyMappings:
    - name: oidc-openid
    - name: oidc-email
    - name: oidc-profile

  signingKeyPair:
    name: self-signed

  subMode: hashed_user_id
  issuerMode: per_provider
  includeClaimsInIDToken: true

  accessCodeValidity: minutes=1
  accessTokenValidity: hours=1
  refreshTokenValidity: days=30

  writeCredentialsTo:
    name: grafana-oidc
```

### Client type

`confidential`

:   The client can keep a secret — a server-side application. authentik
    generates both a client ID and a client secret, and the operator writes both
    to the `Secret` named by `writeCredentialsTo`.

`public`

:   The client cannot keep a secret — a browser SPA or a mobile app. Only a
    client ID is generated. The written `Secret` contains `client-id` alone, with
    no `client-secret` key.

!!! warning "Changing `clientType` is not a small edit"

    Switching `confidential` to `public` discards the client secret. Every
    workload consuming `client-secret` from the generated `Secret` breaks at its
    next restart, and the key simply vanishes rather than becoming empty.

### Redirect URIs

Modern authentik models these as a list of objects with a matching mode rather
than a newline-separated string, and the CRD follows that:

```yaml
redirectURIs:
  - matchingMode: strict
    url: https://grafana.example.com/login/generic_oauth
  - matchingMode: regex
    url: ^https://grafana\.example\.com/.*$
```

!!! danger "`regex` mode is a footgun"

    An unanchored or overly broad pattern lets an attacker redirect the
    authorization code to a host they control, which is a full account
    takeover. Prefer `strict`. If you must use `regex`, anchor both ends and
    escape the dots.

### Generated credentials

See [Credentials](credentials.md) for the full lifecycle. In short: the operator
creates the `Secret` in the provider's own namespace, sets an owner reference so
it is garbage-collected with the provider, and never puts the secret value in
`status`, in an event, or in a log line.

## `SAMLProvider`

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: SAMLProvider
metadata:
  name: awscli
  namespace: my-apps
spec:
  connectionRef:
    name: default

  authorizationFlow:
    name: provider-authorization
  invalidationFlow:
    name: provider-invalidation

  acsURL: https://signin.aws.amazon.com/saml
  audience: urn:amazon:webservices
  issuerOverride: https://authentik.example.com   # optional; EntityID

  signingKeyPair:
    name: self-signed
  verificationKeyPair:              # optional, for signed AuthnRequests
    name: sp-verification

  propertyMappings:
    - name: saml-username
    - name: saml-groups

  nameIDMapping:
    name: saml-email
  digestAlgorithm: http://www.w3.org/2001/04/xmlenc#sha256
  signatureAlgorithm: http://www.w3.org/2001/04/xmldsig-more#rsa-sha256

  assertionValidNotBefore: minutes=-5
  assertionValidNotOnOrAfter: minutes=5
  sessionValidNotOnOrAfter: minutes=86400
```

!!! danger "The signing key is the whole trust relationship"

    Anything holding the private half of `signingKeyPair` can forge assertions
    for every service provider trusting it — sign in as any user, anywhere. The
    operator references the key pair and never reads its private material
    through the API, but the authentik token it holds may be able to. Scope the
    token accordingly.

!!! note "Colons in property mapping names"

    authentik's default SAML mapping names contain a colon, and that name now
    lives on the `PropertyMapping` resource. Quote it, or YAML reads it as a
    mapping key:

    ```yaml
    kind: PropertyMapping
    metadata:
      name: saml-username
    spec:
      connectionRef:
        name: default
      existingName: "authentik default SAML Mapping: Username"
    ```

## `ProxyProvider`

For applications with no SSO support at all. Requests are intercepted by an
[outpost](outposts.md) that authenticates the user and then forwards — or
refuses — the request.

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: ProxyProvider
metadata:
  name: internal-tool
  namespace: my-apps
spec:
  connectionRef:
    name: default

  authorizationFlow:
    name: provider-authorization
  invalidationFlow:
    name: provider-invalidation

  mode: proxy                     # proxy | forward_single | forward_domain
  externalHost: https://tool.example.com
  internalHost: http://internal-tool.my-apps.svc.cluster.local:8080
  internalHostSSLValidation: true

  skipPathRegex: |
    ^/healthz$
    ^/metrics$

  basicAuthEnabled: false
  interceptHeaderAuth: true
```

### Modes

`proxy`

:   The outpost terminates the request and proxies to `internalHost`. Use when
    the outpost is the only way in.

`forward_single`

:   Your existing ingress calls the outpost's auth endpoint for one host.
    `internalHost` is not used.

`forward_domain`

:   As above, but covering a whole domain with one provider. Fewer objects, and
    a correspondingly larger blast radius if the configuration is wrong.

!!! danger "A proxy provider is not wired up until an outpost carries it"

    Creating a `ProxyProvider` configures nothing on its own. It becomes `Ready`
    in authentik's sense while enforcing nothing, because no outpost is serving
    it. Assign it to an [Outpost](outposts.md), or traffic reaches the
    application unauthenticated.

!!! warning "`skipPathRegex` bypasses authentication entirely"

    Every matching path is served with no authentication at all. Anchor the
    patterns. `/health` without anchors also matches
    `/health/../admin/secrets` on some servers.

## Status

All three publish the same [`ManagedResourceStatus`](../reference/api.md):

| Field | Meaning |
| --- | --- |
| `status.conditions` | `Ready` and `Synced`. |
| `status.observedGeneration` | The `metadata.generation` this status reflects. |
| `status.remoteID` | authentik's own identifier — a **numeric primary key** for providers. |
| `status.remoteName` | The name last observed in authentik. Informational. |
| `status.adopted` | Whether this resource took over a pre-existing object. |
| `status.lastSyncedTime` | Last successful reconcile. |

!!! tip "`status.remoteID` is the authoritative handle"

    Once set, lookups prefer it over the name. Renaming the provider on either
    side therefore does not cause the operator to lose track of the object and
    create a duplicate.

    It is also what an `Application` binds to — see
    [Applications](applications.md#the-provider-is-an-int32-primary-key).

## Adoption

`adoptionPolicy` defaults to `FailOnConflict`. If an authentik provider with the
target name already exists and the operator cannot establish that it created it,
the reconcile fails with `AdoptionConflict`, **modifies nothing**, and requeues.

Set `adoptionPolicy: AdoptExisting` to take it over deliberately.
[ADR 0002](../decisions/0002-adoption-policy.md) explains why this is not the
default.

!!! warning "Do not template `AdoptExisting` across everything"

    It is per-object precisely so importing one legacy provider does not lower
    the bar for the rest. A blanket setting applied by a Helm template recreates
    exactly the risk the default exists to prevent.

## See also

- [Applications](applications.md) — binding a provider to something users can click.
- [Outposts](outposts.md) — required for proxy providers.
- [Credentials](credentials.md) — what happens to generated client secrets.
- [Conditions](../reference/conditions.md) — every reason, with fixes.
