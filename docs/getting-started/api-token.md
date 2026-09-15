# Creating an authentik API token

This is the step almost everyone gets stuck on, so it gets its own page. You
need two things: a token created inside authentik, and a Kubernetes `Secret`
holding it.

## 1. Create a service account in authentik

Do not reuse a human administrator's token. When that person leaves, changes
their password, or has their session revoked, your operator stops working — and
in the meantime every action the operator takes is attributed to them.

1. Log in to authentik as an administrator.
2. Go to **Directory → Users** and click **Create**.
3. Set **Username** to something obvious, for example `svc-authentik-operator`.
4. Set **User type** to **Service account**.
5. Save.

Then grant it permissions. Give it the narrowest set that covers the kinds you
actually use — if you never create outposts, it does not need outpost
permissions.

!!! danger "A token is as powerful as the account behind it"

    An admin-group token lets the operator — and anyone who can read the
    `Secret` — create users, rewrite flows, and mint access to every application
    authentik fronts. Scope it deliberately. See
    [Security](../operations/security.md).

## 2. Create the token

1. Go to **Directory → Tokens and App passwords**.
2. Click **Create**.
3. Set **Identifier** to something you will recognise later, e.g.
   `authentik-operator`.
4. Set **User** to the service account you just created.
5. Set **Intent** to **API Token**. This matters — an *App password* is a
   different thing and will not authenticate API calls.
6. Leave **Expiring** unchecked, or set an expiry and plan to rotate. An expired
   token surfaces as a `Ready=False` condition with reason `APIError`, not as
   anything more obvious.
7. Save.

The token value is not shown in the list. Open the token you just created and
use the **copy** action to put it on your clipboard.

## 3. Put the token in a Secret

The operator reads the token from a `Secret` key. Nothing about the `Secret`'s
name or key is fixed — you name both, and the connection object points at them.

```sh
kubectl create secret generic authentik-api-token \
  --namespace my-apps \
  --from-literal=token='ak-your-token-value-here'
```

!!! tip "Keep the token out of your shell history"

    Single quotes stop the shell interpreting characters in the token, but the
    command still lands in `~/.zsh_history`. Prefer reading from a file:

    ```sh
    kubectl create secret generic authentik-api-token \
      --namespace my-apps \
      --from-file=token=./token.txt
    ```

    `--from-file` uses the file's **exact** bytes, so strip the trailing
    newline: `printf %s "$TOKEN" > token.txt`. A newline inside the token value
    is a common cause of a mystifying 403.

Applying a manifest instead:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: authentik-api-token
  namespace: my-apps
type: Opaque
stringData:
  token: ak-your-token-value-here
```

`stringData` takes plain text; `data` requires base64. Do not commit either to
git — use a sealed-secret, an ExternalSecret, or SOPS.

## 4. Point a connection at it

=== "Namespaced (recommended)"

    The token `Secret` must be in the **same namespace** as the connection.
    There is no `namespace` field, deliberately.

    ```yaml
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
    ```

=== "Cluster-scoped"

    The namespace is **required**, and is the only place the token is read
    from. See the warning below.

    ```yaml
    apiVersion: authentik.k8s.rka.sh/v1alpha1
    kind: ClusterAuthentikConnection
    metadata:
      name: shared
    spec:
      url: https://authentik.example.com
      tokenSecretRef:
        namespace: authentik-operator-system
        name: authentik-api-token
        key: token
      allowedNamespaces:
        - my-apps
        - platform
    ```

!!! danger "Creating a `ClusterAuthentikConnection` is a cluster-admin privilege"

    It can name **any** namespace in `tokenSecretRef.namespace`, which means
    whoever can create one can make the operator read any `Secret` in the
    cluster. Read [Connections](../guides/connections.md) first.

### The URL

`spec.url` is the base URL of your authentik instance:

- **Do** include the scheme. The field is validated against `^https?://`.
- **Do not** include the `/api/v3` suffix. The operator appends it.
- **Do not** include a trailing path.

| Value | Verdict |
| --- | --- |
| `https://authentik.example.com` | Correct |
| `https://authentik.example.com/api/v3` | Wrong — the operator appends its own suffix |
| `authentik.example.com` | Rejected by the CRD schema |
| `http://authentik.internal:9000` | Accepted, but unencrypted — the token crosses the network in a header |

## 5. Check it worked

```sh
kubectl -n my-apps get authentikconnection default -o wide
```

The printer columns show the URL, the detected authentik version, `Ready`, and
age. For detail:

```sh
kubectl -n my-apps describe authentikconnection default
```

A healthy connection reports `Ready=True` with reason `Succeeded`, populates
`status.authentikVersion`, and sets `status.versionSupported: true`.

### When it does not

| Symptom | Likely cause |
| --- | --- |
| `Ready=False`, reason `APIError`, message mentions 403 | Wrong token, wrong intent (App password rather than API Token), expired token, or a trailing newline in the `Secret` value. |
| `Ready=False`, reason `APIError`, message mentions connection refused or timeout | Wrong URL, DNS the cluster cannot resolve, or a `NetworkPolicy` blocking egress. |
| `Ready=False`, reason `APIError`, message mentions certificate | Private CA. Set `caBundleSecretRef` rather than reaching for `insecureSkipTLSVerify`. |
| `Ready=False`, reason `ReferenceNotFound` | The `Secret` or its key does not exist in the namespace the operator looked in. |
| `Ready=False`, reason `UnsupportedVersion` | Your authentik is outside the [supported range](../operations/supported-versions.md). |
| `status` is completely empty | The operator is not watching this namespace, or no controller is running. |

Full list at [Conditions](../reference/conditions.md), with fixes at
[Troubleshooting](../operations/troubleshooting.md).

## Private certificate authorities

If authentik presents a certificate from an internal CA, give the operator the
CA bundle rather than disabling verification:

```yaml
spec:
  url: https://authentik.internal.example.com
  tokenSecretRef:
    name: authentik-api-token
    key: token
  caBundleSecretRef:
    name: internal-ca
    key: ca.crt
```

`insecureSkipTLSVerify: true` exists for local testing against a self-signed
instance. Using it in production means the API token can be captured by anything
that can intercept the connection.

## Rotating the token

The operator rereads its `Secret`, so rotation is a `Secret` update:

1. Create a new token in authentik, under the same service account.
2. Update the `Secret`.
3. Confirm the connection is still `Ready`, and `status.lastProbeTime` has
   advanced.
4. Delete the old token in authentik.

`spec.probeInterval` (default `5m`) controls how often reachability is
re-checked, so allow up to that long for the change to be reflected.

## Next

[Declare your first provider and application](first-application.md).
