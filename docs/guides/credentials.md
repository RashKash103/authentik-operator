# Generated credentials

When authentik creates an OAuth2 provider it generates a client ID and, for
confidential clients, a client secret. Those values exist only inside authentik
until something copies them out — and copying them out safely is the whole job
of this page.

## The problem

A workload needs the client secret. The obvious ways to get it there are all
bad:

- **Copy it by hand.** Works once, then rots. Nobody remembers where it came
  from when it needs rotating.
- **Put it in `status`.** Convenient and catastrophic — `status` is readable by
  anything with `get` on the resource, and `get providers` is not usually
  treated as a credential-bearing permission.
- **Log it.** Operator logs are shipped to a log aggregator, indexed, and
  retained for a year, usually with much broader read access than the cluster.

So the operator writes them to a `Secret`, and to nowhere else.

## How it works

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
  clientType: confidential
  writeCredentialsTo:
    name: grafana-oidc
```

On a successful reconcile the operator creates or updates `grafana-oidc` in
`my-apps`:

| Key | Present when | Contents |
| --- | --- | --- |
| `client-id` | Always | The OAuth2 client ID authentik generated. |
| `client-secret` | `clientType: confidential` | The OAuth2 client secret. Absent entirely for public clients. |
| `issuer` | Always | The issuer URL, so an OIDC client can discover the rest. |

```sh
$ kubectl -n my-apps get secret grafana-oidc -o jsonpath='{.data}' | jq 'keys'
[
  "client-id",
  "client-secret",
  "issuer"
]
```

Rename any of them with `clientIDKey`, `clientSecretKey` and `issuerKey` under
`writeCredentialsTo` — useful when a chart expects particular key names.

### Rules the operator follows

!!! success "Same namespace, always"

    The `Secret` is created in the provider's own namespace.
    `credentialsSecretRef` has **no** `namespace` field.

    A cross-namespace write would let anyone who can create a provider in their
    own namespace plant a `Secret` in someone else's — a way to overwrite a
    `Secret` a workload elsewhere depends on. Same reasoning as
    [connections](connections.md).

!!! success "Owned by the provider"

    The `Secret` carries an owner reference to the `OAuth2Provider`, so deleting
    the provider garbage-collects it. No orphaned credentials accumulate.

!!! success "Never in `status`, events, or logs"

    The operator records the `Secret`'s **name** in `status` and in events, and
    never its contents. Condition messages carrying an authentik API error are
    scrubbed before being written, because authentik error bodies occasionally
    echo a submitted value back.

!!! success "Only the keys it manages"

    Updating the `Secret` replaces `clientID` and `clientSecret` and leaves any
    other key untouched. You can keep extra keys — an issuer URL, a
    pre-rendered config file — in the same `Secret`, and the operator will not
    remove them.

### Pre-existing Secrets

If a `Secret` with that name already exists and the operator does not own it,
the reconcile fails with `AdoptionConflict` rather than overwriting it. This is
the same principle as [adoption of authentik objects](../decisions/0002-adoption-policy.md),
applied to Kubernetes: silently taking over a `Secret` somebody else manages is
how an operator breaks an unrelated workload.

## Consuming the credentials

=== "Environment variables"

    ```yaml
    env:
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
    ```

    Simple, and read **once at Pod start**. A rotated secret is not picked up
    until the Pod restarts.

=== "Mounted volume"

    ```yaml
    volumes:
      - name: oidc
        secret:
          secretName: grafana-oidc
    volumeMounts:
      - name: oidc
        mountPath: /etc/oidc
        readOnly: true
    ```

    The kubelet refreshes mounted `Secret` contents (typically within a minute).
    If your application re-reads the file, rotation needs no restart. Most do
    not — check before relying on it.

!!! warning "`envFrom` puts both keys in the environment"

    `envFrom.secretRef` injects every key, including `clientSecret`, under its
    own name. Anything that dumps the environment on a crash — many language
    runtimes do — writes the client secret into a stack trace.

## Rotation

Rotating the client secret is a two-sided operation: authentik must generate a
new one, and every consumer must pick it up.

!!! danger "Rotation causes downtime unless you sequence it"

    The old secret stops working the moment the new one is generated. Any Pod
    still holding the old value fails authentication until it restarts.

The procedure:

1. Trigger regeneration. In authentik, edit the provider and generate a new
   client secret.
2. Wait for the operator to reconcile — up to the connection's `probeInterval`
   (default `5m`) — or force it:

    ```sh
    kubectl -n my-apps annotate oauth2provider grafana \
      authentik.k8s.rka.sh/force-sync="$(date +%s)" --overwrite
    ```

3. Confirm the `Secret` changed:

    ```sh
    kubectl -n my-apps get secret grafana-oidc -o jsonpath='{.metadata.resourceVersion}'
    ```

4. Restart consumers:

    ```sh
    kubectl -n my-apps rollout restart deploy/grafana
    ```

!!! tip "Automate step 4"

    A Reloader-style controller that watches the `Secret` and restarts
    Deployments referencing it removes the manual step, which is the one people
    forget. Without it, rotation looks successful and logins start failing at
    the next unrelated restart — hours or days later, with no obvious cause.

### What the operator does not do

- **It does not rotate on a schedule.** There is no `rotationInterval`. Rotation
  is initiated in authentik or by a human.
- **It does not restart your workloads.** Owner references do not exist between
  a `Secret` and its unrelated consumers.
- **It does not keep the previous secret.** There is no overlap window; the
  `Secret` holds one value.

## Deletion

`deletionPolicy` on the provider governs the authentik side. The Kubernetes
`Secret` is always garbage-collected with the provider, because it is owned by
it.

| Provider deleted with | authentik provider | Generated `Secret` |
| --- | --- | --- |
| `deletionPolicy: Delete` (default) | Deleted | Deleted |
| `deletionPolicy: Orphan` | Kept | Deleted |

!!! warning "`Orphan` keeps the provider but loses your copy of the secret"

    The authentik provider survives with its client secret intact, but the
    Kubernetes `Secret` is garbage-collected. If you still need those values,
    copy them out **before** deleting the resource:

    ```sh
    kubectl -n my-apps get secret grafana-oidc -o yaml > grafana-oidc.backup.yaml
    ```

    Otherwise you will be regenerating the secret in authentik and reconfiguring
    the consumer by hand.

## Hardening

- **Encrypt `Secret`s at rest.** These are live credentials for an identity
  provider. [Encryption at rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/)
  is the baseline.
- **Scope `get secrets`.** In the namespace holding generated credentials, limit
  it to the workloads that need them. `get secrets` on a namespace is read
  access to every client secret in it.
- **Watch what leaves the cluster.** GitOps tooling, backup jobs and log
  shippers that sync or index `Secret`s extend the blast radius wherever they
  send them.
- **Prefer public clients where the architecture allows.** A browser SPA using
  PKCE has no client secret to leak. `clientType: public` is a smaller attack
  surface than `confidential` plus careful secret handling.

See [Security](../operations/security.md) for the full model.

## See also

- [Providers](providers.md#oauth2provider) — the full `OAuth2Provider` spec.
- [Your first application](../getting-started/first-application.md) — end-to-end example.
- [Security](../operations/security.md) — blast radius of each credential.
