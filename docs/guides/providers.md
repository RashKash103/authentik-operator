# Providers

A provider is *how* authentication happens: an OAuth2/OIDC client, a SAML
service provider, or a proxy provider fronted by an outpost. An
[Application](applications.md) points at exactly one.

!!! warning "Planned design — nothing here exists yet"

    `OAuth2Provider`, `SAMLProvider` and `ProxyProvider` have no Go types, no
    CRDs and no controller. This page describes the intended design so it can be
    reviewed before it is built. Manifests here will be rejected by the API
    server today.

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

## References are resolved by name, not UUID

This is the part of the design most worth reviewing, because it is where the
Kubernetes model and authentik's model genuinely disagree.

authentik's API identifies flows, property mappings and certificate keypairs by
**UUID**. Those UUIDs are generated per instance. A manifest containing one is
therefore:

- **not portable** — the same flow has different UUIDs in your staging and
  production authentik, so the same manifest cannot be applied to both;
- **unreviewable** — `ba7ded7b-...` in a pull request tells a reviewer nothing;
- **fragile** — recreating a flow changes its UUID and silently breaks every
  manifest referencing it.

So the CRDs reference the human identifier and the operator resolves it on every
reconcile:

| Reference | Written as | Resolved to | Example |
| --- | --- | --- | --- |
| Flow | **slug** | Flow UUID | `default-provider-authorization-implicit-consent` |
| Property mapping | **name** | Mapping UUID | `goauthentik.io/providers/oauth2/scope-email` |
| Certificate keypair | **name** | Keypair UUID | `authentik Self-signed Certificate` |

### Resolution failures

Resolution happens before anything is written, so a failure never leaves a
half-configured provider in authentik.

`ReferenceNotFound`

:   Nothing in authentik matched. Usually a typo, or a flow that exists in
    staging but was never created in production. The condition message names
    what was being looked for. The resource requeues with backoff, so creating
    the missing flow fixes it without re-applying anything.

`ReferenceAmbiguous`

:   More than one object matched. **Property mapping names are not unique in
    authentik**, so two mappings can legitimately share a name. The operator
    refuses to guess.

!!! danger "Why ambiguity is a hard failure"

    Picking the first match would bind a scope mapping the author did not mean,
    and the symptom would be a token missing a claim — noticed weeks later, by
    an application behaving subtly wrong. There is no safe guess, so the
    operator stops and names both candidates in the condition message.

    Fix it by renaming one of the mappings in authentik so the reference is
    unambiguous.

!!! note "Resolution costs API calls"

    Every reconcile resolves every reference. With many providers sharing the
    same flows this is repetitive, so resolution results are cached per
    connection for the duration of a reconcile. A flow renamed in authentik is
    picked up on the next reconcile, not instantly.

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

  authorizationFlow: default-provider-authorization-implicit-consent
  invalidationFlow: default-provider-invalidation-flow

  clientType: confidential          # or public
  redirectURIs:
    - matchingMode: strict
      url: https://grafana.example.com/login/generic_oauth

  propertyMappings:
    - goauthentik.io/providers/oauth2/scope-openid
    - goauthentik.io/providers/oauth2/scope-email
    - goauthentik.io/providers/oauth2/scope-profile

  signingKey: authentik Self-signed Certificate

  subMode: hashed_user_id
  issuerMode: per_provider
  includeClaimsInIDToken: true

  accessCodeValidity: minutes=1
  accessTokenValidity: hours=1
  refreshTokenValidity: days=30

  credentialsSecretRef:
    name: grafana-oidc
```

### Client type

`confidential`

:   The client can keep a secret — a server-side application. authentik
    generates both a client ID and a client secret, and the operator writes both
    to `credentialsSecretRef`.

`public`

:   The client cannot keep a secret — a browser SPA or a mobile app. Only a
    client ID is generated. The written `Secret` contains `clientID` alone, with
    no `clientSecret` key.

!!! warning "Changing `clientType` is not a small edit"

    Switching `confidential` to `public` discards the client secret. Every
    workload consuming `clientSecret` from the generated `Secret` breaks at its
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

  authorizationFlow: default-provider-authorization-implicit-consent
  invalidationFlow: default-provider-invalidation-flow

  acsURL: https://signin.aws.amazon.com/saml
  audience: urn:amazon:webservices
  issuer: https://authentik.example.com

  signingKey: authentik Self-signed Certificate     # resolved by name
  verificationKP: ""                                 # optional, for signed AuthnRequests

  propertyMappings:
    - authentik default SAML Mapping: Username
    - authentik default SAML Mapping: Groups

  nameIDMapping: authentik default SAML Mapping: Email
  digestAlgorithm: http://www.w3.org/2001/04/xmlenc#sha256
  signatureAlgorithm: http://www.w3.org/2001/04/xmldsig-more#rsa-sha256

  assertionValidNotBefore: minutes=-5
  assertionValidNotOnOrAfter: minutes=5
  sessionValidNotOnOrAfter: minutes=86400
```

!!! danger "The signing key is the whole trust relationship"

    Anything holding the private half of `signingKey` can forge assertions for
    every service provider trusting it — sign in as any user, anywhere. The
    operator references the keypair by name and never reads its private material
    through the API, but the authentik token it holds may be able to. Scope the
    token accordingly.

!!! note "Colons in property mapping names"

    authentik's default SAML mapping names contain a colon
    (`authentik default SAML Mapping: Username`). In YAML that must be quoted
    when it appears as a scalar that could be read as a mapping key. Prefer:

    ```yaml
    propertyMappings:
      - "authentik default SAML Mapping: Username"
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

  authorizationFlow: default-provider-authorization-implicit-consent
  invalidationFlow: default-provider-invalidation-flow

  mode: forward_single            # proxy | forward_single | forward_domain
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
