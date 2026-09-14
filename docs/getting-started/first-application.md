# Your first application

An end-to-end example: a connection, an OAuth2 provider, an application, and a
workload consuming the generated credentials. We will use Grafana, because its
OIDC configuration is short enough to fit on a page.

!!! warning "This is a design document, not a tutorial"

    `OAuth2Provider` and `Application` **do not exist yet** — no Go types, no
    CRDs, no controller. The manifests below will be rejected by the API server
    today.

    They are published so the API shape can be reviewed before it is built. The
    two connection kinds do exist as Go types, but nothing reconciles them
    either. See [Implementation status](index.md#implementation-status).

## What we are building

```text
  Secret: authentik-api-token          your authentik API token
        │
        ▼
  AuthentikConnection/default          which authentik, and with what token
        │
        ▼
  OAuth2Provider/grafana  ──────────▶  Secret: grafana-oidc
        │                                 (clientID, clientSecret — written by
        ▼                                  the operator, consumed by Grafana)
  Application/grafana                  what users click in the authentik library
```

## 0. Prerequisites

The operator installed ([Installation](installation.md)), and a `Secret` holding
an authentik API token ([Creating an API token](api-token.md)):

```sh
kubectl create namespace my-apps
kubectl create secret generic authentik-api-token \
  --namespace my-apps \
  --from-file=token=./token.txt
```

## 1. The connection

```yaml title="connection.yaml"
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: AuthentikConnection
metadata:
  name: default
  namespace: my-apps
spec:
  url: https://authentik.example.com
  tokenSecretRef:
    name: authentik-api-token
    key: token
  probeInterval: 5m
```

```sh
kubectl apply -f connection.yaml
kubectl -n my-apps wait --for=condition=Ready authentikconnection/default --timeout=60s
```

Do not go further until this is `Ready`. Everything downstream refuses to
reconcile against a connection that is not, and reports `ConnectionNotReady`
rather than a useful error of its own.

```sh
$ kubectl -n my-apps get akconn
NAME      URL                             VERSION    READY   AGE
default   https://authentik.example.com   2026.8.2   True    12s
```

## 2. The OAuth2 provider

```{ .yaml .annotate title="provider.yaml" }
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: OAuth2Provider
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    kind: AuthentikConnection      # (1)!
    name: default

  adoptionPolicy: FailOnConflict   # (2)!
  deletionPolicy: Delete

  authorizationFlow: default-provider-authorization-implicit-consent  # (3)!
  invalidationFlow: default-provider-invalidation-flow

  clientType: confidential
  redirectURIs:
    - matchingMode: strict
      url: https://grafana.example.com/login/generic_oauth

  propertyMappings:                # (4)!
    - goauthentik.io/providers/oauth2/scope-openid
    - goauthentik.io/providers/oauth2/scope-email
    - goauthentik.io/providers/oauth2/scope-profile

  signingKey: authentik Self-signed Certificate   # (5)!

  accessTokenValidity: hours=1
  refreshTokenValidity: days=30

  credentialsSecretRef:            # (6)!
    name: grafana-oidc
```

1. Optional — `AuthentikConnection` is the default. Set it to
   `ClusterAuthentikConnection` to use a cluster-scoped connection.
2. The default. The reconcile fails rather than taking over a pre-existing
   authentik provider named `grafana`. See
   [ADR 0002](../decisions/0002-adoption-policy.md).
3. Flows are referenced by **slug**, not by UUID. The operator resolves the slug
   against the connection's authentik on every reconcile.
4. Property mappings are referenced by **name**. The operator resolves each to
   its UUID.
5. The signing keypair is referenced by **name**.
6. Where the operator writes the generated `clientID` and `clientSecret`. The
   `Secret` is created in this resource's own namespace.

### Names and slugs, not UUIDs

authentik's API identifies flows, property mappings and certificate keypairs by
UUID. Those UUIDs are generated per-instance, so a manifest containing one is
not portable between your staging and production authentik — and is unreadable
in review.

So the CRDs reference these by the human identifier and the operator resolves it
on each reconcile:

| Referenced by | Resolved to |
| --- | --- |
| Flow — by **slug** | Flow UUID |
| Property mapping — by **name** | Mapping UUID |
| Certificate keypair — by **name** | Keypair UUID |

The failure modes get their own condition reasons:

- No match — `ReferenceNotFound`. Usually a typo, or a flow that exists in
  staging but was never created in production.
- More than one match — `ReferenceAmbiguous`. Property mapping names are not
  unique in authentik, so this is a real possibility. The operator refuses to
  guess rather than picking one and silently binding the wrong mapping.

## 3. The application

```{ .yaml .annotate title="application.yaml" }
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: Application
metadata:
  name: grafana
  namespace: my-apps
spec:
  connectionRef:
    name: default

  adoptionPolicy: FailOnConflict
  deletionPolicy: Delete

  slug: grafana                    # (1)!
  displayName: Grafana
  group: Observability             # (2)!

  providerRef:                     # (3)!
    kind: OAuth2Provider
    name: grafana

  meta:
    description: Dashboards and alerting
    launchURL: https://grafana.example.com
    publisher: Platform team

  policyEngineMode: any
```

1. The slug is authentik's identity for the application and appears in its URLs.
   Changing it later renames the object in authentik.
2. Optional. Groups applications in the user-facing library page.
3. A reference to a provider resource in this namespace — not to an authentik
   primary key.

### Ordering does not matter

`Application.provider` in authentik's API is an **int32 primary key**, which
only exists once the provider has actually been created. So an `Application`
applied before its provider cannot be fully created.

It does not fail. It waits:

- The `Application` is admitted and reconciled.
- The provider reference does not resolve yet, so it reports `Ready=False` with
  reason `ReferenceNotFound` and requeues with backoff.
- When the `OAuth2Provider` becomes ready and publishes its numeric
  `status.remoteID`, the `Application` reconciles again and converges.

This matters for GitOps, where a whole directory is applied at once in
whatever order the tool chose. `kubectl apply -f .` converges; it does not
require you to sequence the files.

```sh
kubectl apply -f provider.yaml -f application.yaml
kubectl -n my-apps wait --for=condition=Ready application/grafana --timeout=120s
```

!!! tip "Watching convergence"

    ```sh
    kubectl -n my-apps get application grafana -w
    ```

    Seeing `ReferenceNotFound` for a few seconds after a bulk apply is normal.
    Seeing it for minutes means the provider itself is not becoming ready —
    check the provider, not the application.

## 4. Consume the credentials

The operator creates `grafana-oidc` in `my-apps` with two keys:

| Key | Contents |
| --- | --- |
| `clientID` | The OAuth2 client ID authentik generated |
| `clientSecret` | The OAuth2 client secret authentik generated |

```yaml title="grafana-deployment.yaml"
env:
  - name: GF_AUTH_GENERIC_OAUTH_ENABLED
    value: "true"
  - name: GF_AUTH_GENERIC_OAUTH_CLIENT_ID
    valueFrom:
      secretKeyRef:
        name: grafana-oidc
        key: clientID
  - name: GF_AUTH_GENERIC_OAUTH_CLIENT_SECRET
    valueFrom:
      secretKeyRef:
        name: grafana-oidc
        key: clientSecret
  - name: GF_AUTH_GENERIC_OAUTH_AUTH_URL
    value: https://authentik.example.com/application/o/authorize/
  - name: GF_AUTH_GENERIC_OAUTH_TOKEN_URL
    value: https://authentik.example.com/application/o/token/
  - name: GF_AUTH_GENERIC_OAUTH_API_URL
    value: https://authentik.example.com/application/o/userinfo/
```

!!! warning "Environment variables do not reload"

    A Pod reads `secretKeyRef` values once, at start. If the client secret is
    ever regenerated, the Pod keeps the stale value until it restarts. See
    [Credentials](../guides/credentials.md) for the rotation procedure.

## 5. Verify

```sh
kubectl -n my-apps get akconn,oauth2provider,application
kubectl -n my-apps get secret grafana-oidc
```

Then open your authentik library page. Grafana should appear under
**Observability**, and clicking it should land you in a logged-in Grafana.

If it does not:

```sh
kubectl -n my-apps describe application grafana
kubectl -n my-apps describe oauth2provider grafana
kubectl -n my-apps get events --sort-by=.lastTimestamp
```

Work bottom-up — connection, then provider, then application. A failure at one
layer shows up as `ConnectionNotReady` or `ReferenceNotFound` at the layer
above, and chasing the symptom rather than the cause wastes time.

## Cleaning up

```sh
kubectl delete -f application.yaml -f provider.yaml
```

With `deletionPolicy: Delete` (the default) the authentik objects go too. Set
`deletionPolicy: Orphan` to keep them — useful when handing an object back to
manual management, or when tearing down a cluster whose authentik outlives it.

The generated `grafana-oidc` `Secret` is owned by the `OAuth2Provider` and is
garbage-collected with it.

## Next

- [Connections](../guides/connections.md) — choosing between the two kinds.
- [Providers](../guides/providers.md) — SAML and proxy providers, and reference resolution in depth.
- [Credentials](../guides/credentials.md) — rotation, and why secrets never appear in `status`.
- [Troubleshooting](../operations/troubleshooting.md) — when the above does not work.
