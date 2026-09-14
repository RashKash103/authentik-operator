# Security Policy

> [!WARNING]
> **This project is fully AI generated.** No part of it has been through an
> independent human security audit. Its threat model, its RBAC, and its handling
> of credentials were all written by a language model. Please review it before
> pointing it at an authentik instance that matters, and treat the guarantees
> below as intentions rather than as verified properties. See the note on
> [AI provenance](#ai-generation-provenance) at the end of this document.

This operator is security-adjacent by construction. It holds authentik API
tokens, it can hold kubeconfigs for remote clusters, and it generates OAuth2
client secrets. Compromising it is close to compromising the identity provider
it configures.

## Supported versions

The project is pre-1.0 and the API group `authentik.k8s.rka.sh/v1alpha1` is
unstable. Security fixes land on `main` and in the next tagged release; there
are **no backports to older tags** and no long-term support branches.

| Version   | Security fixes                          |
| --------- | --------------------------------------- |
| `main`    | Yes                                     |
| Latest release tag | Yes, via the next release      |
| Any older tag | No — upgrade to the latest release  |

For authentik itself, the operator supports the series listed in
[`supported-versions.yaml`](supported-versions.yaml) and reproduced in the
[README](README.md#supported-authentik-versions). Vulnerabilities in authentik
should go to the [authentik project](https://goauthentik.io/), not here.

## Reporting a vulnerability

**Please do not open a public issue for a security vulnerability.** Public
issues are visible to everyone the moment they are filed, including to anyone
running an affected deployment who has not yet patched.

Report privately through **GitHub private security advisories**: go to the
repository's **Security → Advisories → Report a vulnerability** page. That
creates a private thread visible only to the maintainers and to you.

<!-- PLACEHOLDER: replace with the repository's real advisory URL once the
     repository location is fixed, e.g.
     https://github.com/OWNER/authentik-operator/security/advisories/new -->

Please include:

- The affected version or commit.
- A description of the impact — what an attacker gains, and what access they
  need to start with.
- Reproduction steps or a proof of concept, if you have one.
- Any suggested fix.

### What to expect

| Stage                          | Target                                   |
| ------------------------------ | ---------------------------------------- |
| Acknowledgement of your report | Within 5 business days                   |
| Initial assessment and severity | Within 10 business days                 |
| Fix or mitigation plan          | Communicated with the assessment         |
| Public advisory                 | After a fix ships, coordinated with you  |

<!-- PLACEHOLDER: these targets are a stated intention, not an SLA backed by an
     on-call rotation. The owner should adjust them to what is actually
     sustainable, or state plainly that this is a best-effort personal project. -->

This is a small project. If you have not heard back within the acknowledgement
window, please reply on the advisory thread rather than escalating to a public
issue.

We do not run a bug bounty and cannot offer payment.

## Security model

### What the operator can access

The controller-manager runs as a single `Deployment` in its own namespace, as a
non-root user with a read-only root filesystem, `allowPrivilegeEscalation:
false`, all Linux capabilities dropped, and the `RuntimeDefault` seccomp
profile. Within those limits it has four kinds of reach:

1. **The Kubernetes API**, via its ServiceAccount. It watches its own custom
   resources across all namespaces by default (narrow this with
   `--watch-namespaces`), reads `Secret`s referenced by connection objects, and
   writes `Secret`s containing generated credentials.
2. **The authentik REST API**, via tokens it reads from `Secret`s. Whatever the
   token can do, the operator can do.
3. **Remote clusters and Docker hosts**, indirectly. A
   `KubernetesServiceConnection` or `DockerServiceConnection` hands authentik
   the credentials it needs to deploy managed outposts; the operator reads those
   credentials from `Secret`s in order to pass them on.
4. **The network between the operator and authentik**, including whatever
   `spec.url` points at. A connection object is a request for the operator to
   make an authenticated outbound HTTP call to an address the object's author
   chose.

### Blast radius per credential

| Credential                            | Held where                                       | If it leaks                                                                                                                             |
| ------------------------------------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| authentik API token                   | A `Secret` named by a connection object          | Full control of authentik to the limit of the token's own permissions. A token from an admin account can create users, reset flows, and mint access to every application authentik fronts. |
| Generated OAuth2 client secret        | A `Secret` the operator writes                   | Impersonation of that OAuth2 client. Depending on the flow and the relying party's configuration, that can mean signing in as arbitrary users of that application. |
| SAML signing key material             | A `Secret` referenced by a `SAMLProvider`        | Forged SAML assertions for that provider — sign-in as any user, at any service trusting it.                                               |
| Kubeconfig (`KubernetesServiceConnection`) | A `Secret` referenced by the connection     | Whatever that kubeconfig grants in the target cluster. Outpost deployment requires creating workloads, so these are rarely low-privilege. |
| Docker host credentials (`DockerServiceConnection`) | A `Secret` referenced by the connection | Control of the Docker daemon, which on most hosts is equivalent to root on that host.                                                     |
| The operator's own ServiceAccount token | Projected into the manager Pod                 | Read access to every `Secret` the operator is permitted to read, plus the ability to rewrite the custom resources it manages.             |

The operator never logs token or secret values. It records `Secret` names and
namespaces in events, conditions and logs, which are themselves useful to an
attacker mapping the cluster — treat operator logs as sensitive.

### `ClusterAuthentikConnection` is a cluster-admin-level privilege

This is the sharpest edge in the design, and it is deliberate rather than
accidental — read it before you grant anyone access.

A `ClusterAuthentikConnection` is cluster-scoped and names, in its own spec, the
namespace its API token `Secret` lives in:

```yaml
apiVersion: authentik.k8s.rka.sh/v1alpha1
kind: ClusterAuthentikConnection
metadata:
  name: shared
spec:
  url: https://authentik.example.com
  tokenSecretRef:
    namespace: kube-system   # <- any namespace in the cluster
    name: some-secret
    key: token
```

The operator has to be able to read `Secret`s cluster-wide to honour that field.
Therefore:

> [!CAUTION]
> **Anyone who can create or update a `ClusterAuthentikConnection` can cause the
> operator to read a `Secret` from any namespace in the cluster.** Combined with
> a `spec.url` under their control — a URL they own, or any endpoint reachable
> from the operator Pod — they can have the operator send the contents of that
> `Secret` to them as a bearer token. Treat `create` and `update` on
> `ClusterAuthentikConnection` as equivalent to cluster-wide `get secrets`,
> which is to say as cluster-admin.

Two mitigations are part of the design, and neither removes the underlying
privilege:

- The token `Secret` is resolved **only** from the namespace named in
  `spec.tokenSecretRef.namespace`. It is never resolved relative to the
  namespace of the resource that references the connection. This closes the
  classic operator privilege-escalation bug where a tenant in namespace `A`
  points at a connection and quietly borrows a `Secret` from namespace `B`.
  See [ADR 0001](docs/decisions/0001-connection-scope.md).
- `AuthentikConnection` — the namespaced variant — resolves its token `Secret`
  only from its own namespace and carries no cross-namespace reach at all. It is
  the default recommendation for anything multi-tenant.

If you do not need a shared cluster-wide authentik endpoint, **do not install
RBAC granting anyone but cluster administrators access to
`ClusterAuthentikConnection`**, and consider removing the CRD entirely.

### Trust boundaries the operator does not cross

- It does not read `Secret`s outside the namespace of the referencing resource,
  except through a `ClusterAuthentikConnection`'s explicitly named namespace.
- It does not accept a connection target from an annotation, a label, or any
  field outside the connection object's own spec.
- It does not manage authentik itself, so it needs no privileges over the
  authentik deployment, its database, or its storage.

## Hardening guidance

### Scope the authentik token

Create a dedicated authentik service account and token for the operator rather
than reusing a human administrator's token. Grant it only the permissions the
kinds you actually use require — if you never create outposts, it does not need
outpost permissions. Rotate the token on a schedule; the operator rereads its
`Secret`, so rotation is a `Secret` update.

Give each tenant or environment its own token and its own `AuthentikConnection`.
A single shared admin token behind a `ClusterAuthentikConnection` collapses
every tenant into one blast radius.

### Restrict who may create connection objects

Connection objects are the privilege boundary. Providers and applications are
comparatively harmless; connections are not.

- Grant `create`/`update`/`patch` on `ClusterAuthentikConnection` to cluster
  administrators only. Do not include it in any role bound to tenants,
  application teams or CI service accounts.
- Grant `AuthentikConnection` per namespace, to the team that owns that
  namespace. Remember that creating one also means choosing which `Secret` in
  that namespace the operator reads, so it is roughly equivalent to `get
  secrets` within that namespace.
- Audit both kinds. A new `ClusterAuthentikConnection` in a cluster that did not
  expect one is a strong signal.
- Consider an admission policy (ValidatingAdmissionPolicy, Kyverno, Gatekeeper)
  that restricts `spec.url` to an allowlist of hostnames and restricts
  `spec.tokenSecretRef.namespace` to a known set. That turns the exfiltration
  path above from "any secret, anywhere it likes" into "a known secret, to a
  known endpoint".

### Limit the operator's own reach

- Run with `--watch-namespaces` set to the namespaces you actually use, rather
  than cluster-wide, where your topology allows it.
- Keep `--metrics-secure=true` (the default) so the metrics endpoint requires
  authentication and authorisation. Leave `--enable-http2=false` (the default);
  it is off deliberately because of the HTTP/2 denial-of-service CVE family.
- Do not add RBAC beyond what the generated `manager-role` requests. In
  particular, resist granting blanket `secrets` `list`/`watch` if a narrower
  `get` is sufficient for your installation.
- Run the operator in its own namespace, with no other workloads, so a namespace
  compromise does not hand over its ServiceAccount token.

### Network policy

The operator needs egress to the Kubernetes API server and to your authentik
endpoint. It needs nothing else, and constraining it is the cheapest available
mitigation for the exfiltration path described above.

- Apply a default-deny egress `NetworkPolicy` to the operator namespace, then
  allow only the API server and the authentik service. If authentik lives in the
  same cluster, target it by namespace and pod selector rather than by IP.
- Allow ingress only from your metrics scraper, on the metrics port, and from
  the kubelet for the probe port.
- With egress locked down, a hostile `spec.url` can no longer reach an
  attacker-controlled endpoint, which blunts the `ClusterAuthentikConnection`
  issue considerably. It does not fix it: a cluster-internal endpoint is still
  reachable.

### Protect the generated secrets

The `Secret`s the operator writes contain live OAuth2 client secrets. Enable
[encryption at rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/)
for `Secret`s, keep `get secrets` in those namespaces scoped to the workloads
that need them, and remember that anything syncing `Secret`s out of the cluster
(GitOps tooling, backup jobs, log shippers) extends the blast radius with it.

## AI generation provenance

Every file in this repository — the operator, its tests, its RBAC, this security
policy, and the threat model it describes — was generated by an AI system. This
matters for security review in specific ways:

- **Plausible-looking security controls may not do anything.** Generated code
  can produce a check that reads correctly and validates nothing. The
  cross-namespace `Secret` resolution rule described above is exactly the kind
  of control worth verifying against the source rather than trusting the prose.
- **The threat model is asserted, not derived.** It was not produced by
  adversarial review of a running system.
- **Dependencies and versions were chosen by generation**, not by a supply-chain
  policy. Check them against your own.

If you are evaluating this project for production use, treat it as unreviewed
third-party code from an anonymous author, because functionally that is what it
is. Reviews and security reports are genuinely welcome.
