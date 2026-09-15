# Troubleshooting

Symptom first, then cause, then fix. If you already have a condition reason,
[Conditions](../reference/conditions.md) explains each one in depth.

## Start here

Three commands, in this order. Most problems are identified by the second.

```sh
# 1. What does the resource itself say?
kubectl -n my-apps describe oauth2provider grafana

# 2. What does its connection say? Failures cascade upward.
kubectl -n my-apps describe authentikconnection default

# 3. What is the operator doing?
kubectl -n authentik-operator-system logs deploy/authentik-operator --tail=100
```

!!! tip "Always work bottom-up"

    Connection → provider → application. A broken connection shows up as
    `ConnectionNotReady` on every provider and `ReferenceNotFound` on every
    application, and chasing the application's condition is time spent
    diagnosing a faithful report of somebody else's problem.

## Nothing happens: `status` is empty

No conditions, no events, no `observedGeneration`. The resource was accepted and
then ignored.

| Cause | Check | Fix |
| --- | --- | --- |
| **The operator predates this kind** | `kubectl -n authentik-operator-system logs deploy/authentik-operator \| grep 'Starting Controller'` | Every kind in the [API reference](../reference/api.md#kinds) has a controller. An older operator image will not start one for a newer CRD. |
| **The namespace is not watched** | `helm get values authentik-operator -n authentik-operator-system` | Add the namespace to `watchNamespaces`, or set it to `[]` for all. |
| **The operator is not running** | `kubectl -n authentik-operator-system get pods` | `CrashLoopBackOff` or `ImagePullBackOff` — read the logs. |
| **Leader election has no leader** | Logs for `successfully acquired lease` | Check the Pod can write `coordination.k8s.io` leases. |
| **The CRD is a leftover from an older version** | `kubectl get crd authentikconnections.authentik.k8s.rka.sh -o yaml` | Helm never upgrades CRDs. [Apply them yourself](../getting-started/installation.md#upgrading-the-chart-crds-are-your-job). |

!!! danger "`watchNamespaces` ignores resources silently"

    No event, no condition, no log line. The operator's cache never contained
    the object. If a resource is completely inert and the operator is otherwise
    healthy, check this value before anything else.

## Fields I set are missing after apply

You applied a manifest, `kubectl get -o yaml` comes back without some fields,
and nothing errored.

**Cause:** the CRD in the cluster is older than the manifest. The API server
prunes unknown fields silently.

```sh
kubectl get crd authentikconnections.authentik.k8s.rka.sh \
  -o jsonpath='{.metadata.annotations}' | jq
```

**Fix:** apply the current CRDs. Helm does not do this on upgrade.

```sh
kubectl apply --server-side -f charts/authentik-operator/crds/
```

Add `--force-conflicts` if an earlier client-side apply left a large
`last-applied-configuration` annotation, which also produces
`metadata.annotations: Too long`.

## The connection will not become Ready

### 401 or 403

By far the most common, and usually not what people expect.

| Cause | Fix |
| --- | --- |
| **A trailing newline in the token** | `--from-literal` keeps exactly what you typed, but a token pasted from a file or an editor often carries `\n`. Rewrite it: `printf %s "$TOKEN" > token.txt` then `--from-file=token=./token.txt`. |
| **Wrong intent** | The token must be created with intent **API Token**. An *App password* is a different object and will not authenticate API calls. |
| **Expired** | Check the token in authentik under Directory → Tokens and App passwords. |
| **The service account lacks permissions** | It needs permissions for the object types you manage, not just to authenticate. |
| **The wrong key** | The `Secret` exists but `key:` names a key it does not have. |

Check what is actually stored:

```sh
kubectl -n my-apps get secret authentik-api-token -o jsonpath='{.data.token}' \
  | base64 -d | xxd | tail -2
```

A trailing `0a` is a newline. That is your problem.

### Connection refused, timeout, or no such host

| Cause | Check |
| --- | --- |
| Wrong URL | `spec.url` must be the bare base URL — scheme required, no `/api/v3`, no trailing path. |
| DNS the cluster cannot resolve | A split-horizon name that resolves on your laptop but not in-cluster. |
| `NetworkPolicy` blocking egress | A default-deny policy in the operator's namespace with no rule for authentik. |
| authentik not actually listening on that port | Especially with a non-standard port. |

Test from inside the cluster, not from your laptop:

```sh
kubectl -n authentik-operator-system run curl --rm -it --restart=Never \
  --image=curlimages/curl -- \
  curl -sS -o /dev/null -w '%{http_code}\n' https://authentik.example.com/api/v3/root/config/
```

### Certificate signed by unknown authority

authentik presents a certificate from a private CA.

```yaml
spec:
  caBundleSecretRef:
    name: internal-ca
    key: ca.crt
```

!!! warning "Do not reach for `insecureSkipTLSVerify`"

    It exists for local testing against a self-signed instance. In production it
    means the API token can be captured by anything able to intercept the
    connection.

### 404 on every call

`spec.url` includes `/api/v3` or another path. The operator appends the suffix
itself; give it `https://authentik.example.com` and nothing more.

### `UnsupportedVersion`

```sh
kubectl -n my-apps get authentikconnection default \
  -o jsonpath='{.status.authentikVersion}{"\n"}'
```

Compare against [Supported versions](../operations/supported-versions.md). Below
the minimum, dependent resources refuse to reconcile. Above the tested maximum,
they reconcile with a warning.

## A provider or application will not become Ready

### `ConnectionNotReady`

The resource is fine; its connection is not. Nothing was written to authentik.

1. `describe` the connection and fix it.
2. Check `connectionRef.kind` — pointing at `AuthentikConnection` when you
   created a `ClusterAuthentikConnection` produces this, since the namespaced
   lookup finds nothing.
3. For a cluster connection, check the referring namespace is in
   `allowedNamespaces`.

### `ReferenceNotFound`

The message names what was missing. Match it to the table:

| Missing | Resolved where |
| --- | --- |
| A `Secret` | The connection's own namespace, or `tokenSecretRef.namespace` for the cluster kind |
| A connection | The referring resource's **own namespace**, always — `connectionRef` never crosses one |
| A provider or service connection | The referring resource's own namespace, or `namespace` if cross-namespace references are enabled |
| A flow, property mapping or certificate keypair | The `Flow`, `PropertyMapping` or `CertificateKeyPair` **resource**, through its `status.remoteID` — never by looking the name up in authentik |

!!! tip "A reference naming another namespace"

    `spec.authorizationFlow: names namespace "platform", but the operator was
    started without --allow-cross-namespace-references` means exactly that:
    either drop the `namespace`, or set the chart value
    `allowCrossNamespaceReferences`. It is refused rather than resolved locally,
    because resolving locally would pick a different object than the manifest
    names.

    A reference that resolves but reports *"a reference must point at the same
    authentik instance"* is a different fault: the two namespaces are wired to
    different authentiks, and a UUID from one means nothing to the other.

!!! success "Ordering resolves itself"

    An `Application` applied before its provider reports `ReferenceNotFound` and
    requeues, then converges when the provider publishes its numeric
    `status.remoteID`. A few seconds of this after `kubectl apply -f .` is
    normal.

    Minutes of it means the **provider** is not becoming ready. Go look at the
    provider.

### `ReferenceAmbiguous`

Two objects in authentik share the name. Property mapping names are not unique,
so this is a real case rather than a corner one. Rename one of them in
authentik; the operator will not guess.

### `AdoptionConflict`

An object with that name or slug already exists and the operator cannot prove it
created it. **Nothing was modified.**

```sh
kubectl -n my-apps get oauth2provider grafana \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}{"\n"}'
```

Then pick one: set `adoptionPolicy: AdoptExisting` to take it over, rename yours
to avoid the collision, or delete the pre-existing object in authentik. See
[ADR 0002](../decisions/0002-adoption-policy.md).

!!! warning "Two namespaces, one slug"

    authentik slugs are global; Kubernetes names are namespaced. `my-apps/grafana`
    and `staging/grafana` both declaring `slug: grafana` collide, and the second
    to reconcile gets this error. Namespace your slugs.

### `InvalidSpec`

authentik rejected the request. The message is authentik's own and names the
field it objects to. Common cases: a malformed redirect URI, a flow of the wrong
designation, a field combination the provider type forbids, or an `Outpost`
whose `providerRefs` do not match its `type`.

## Deletion hangs

The resource sits in `Terminating` with reason `Deleting`.

**Cause:** the finalizer cannot complete, almost always because authentik is
unreachable so the operator cannot confirm the delete.

**Fix:** repair the connection. Deletion completes on its own.

!!! danger "Force-removing the finalizer orphans the authentik object"

    ```sh
    # Last resort only.
    kubectl -n my-apps patch oauth2provider grafana \
      -p '{"metadata":{"finalizers":[]}}' --type=merge
    ```

    The Kubernetes resource disappears; the authentik object remains, managed by
    nobody, and causes `AdoptionConflict` the next time anything claims that
    name. Delete it in authentik yourself.

## Logins fail after a secret rotation

**Cause:** the `Secret` updated; the Pod did not. `secretKeyRef` environment
variables are read once, at start.

```sh
kubectl -n my-apps get secret grafana-oidc -o jsonpath='{.metadata.resourceVersion}{"\n"}'
kubectl -n my-apps rollout restart deploy/grafana
```

See [Credentials](../guides/credentials.md#rotation). A Reloader-style
controller automates the step people forget.

## A proxied application is served without authentication

**Cause:** the `ProxyProvider` is not assigned to any outpost. A proxy provider
enforces nothing on its own.

Check the outpost's `providerRefs` includes it, and that the outpost is actually
running — `Ready=True` on the `Outpost` means authentik accepted the
configuration, **not** that the outpost Pods started and connected.

Also check `skipPathRegex` on the provider: every matching path is served with
no authentication at all, and an unanchored pattern matches far more than
intended.

## Useful commands

```sh
# Everything this operator manages, in one namespace
kubectl -n my-apps get akconn,oauth2provider,samlprovider,proxyprovider,application,outpost

# Recent events, newest last
kubectl -n my-apps get events --sort-by=.lastTimestamp | tail -20

# Just the Ready condition message
kubectl -n my-apps get oauth2provider grafana \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].message}{"\n"}'

# Has the operator seen my latest edit?
kubectl -n my-apps get oauth2provider grafana \
  -o jsonpath='gen={.metadata.generation} observed={.status.observedGeneration}{"\n"}'

# Turn up the logs
helm upgrade authentik-operator ./charts/authentik-operator \
  -n authentik-operator-system --reuse-values --set log.level=debug
```

!!! tip "`generation` versus `observedGeneration`"

    If `observedGeneration` is behind `metadata.generation`, the operator has
    not processed your latest edit and every condition you are reading describes
    the **previous** spec. Wait, or find out why it is not reconciling.

## Still stuck

- [Conditions](../reference/conditions.md) — every reason in depth.
- [Connections](../guides/connections.md) — how references resolve.
- [Security](security.md) — if the question is "should this have been allowed?"
- Report bugs in the issue tracker. **Security vulnerabilities go through
  GitHub private security advisories instead** — see `SECURITY.md` in the
  repository root.
