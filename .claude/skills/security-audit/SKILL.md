---
name: security-audit
description: Run a comprehensive security audit of the authentik-operator codebase - credential handling, RBAC, supply chain, CI/CD, and input validation - reporting findings by severity and fixing them. Use when asked to audit security, review for vulnerabilities, before cutting a release, or after changes touching credentials, RBAC, or workflows.
---

# Security audit

This operator holds authentik API tokens, kubeconfigs and generated OAuth client
secrets, and its CRDs can reference Secrets across a cluster. It is
security-adjacent by nature, so "looks fine" is not a result — each area below
needs a specific check with a specific answer.

Read `SECURITY.md` first: it states the intended threat model. Much of this
audit is verifying the code actually honours it.

## How to run it

Audit the areas below. They are largely independent, so for a full sweep run
them as parallel subagents with read-only instructions, then fix centrally —
concurrent edits to the same packages conflict.

**Read the code. Do not infer from comments.** A comment claiming a protection
is exactly the thing to verify. Several real bugs here were found only by
running something, not by reading it.

Distinguish **confirmed** findings from **theoretical** ones, and say so. Do not
manufacture findings to appear thorough; a clean result on a named check is
useful information.

## Areas

### 1. Credential leakage

Trace every path a secret value could reach an observable surface: log lines,
Kubernetes Events, condition messages, `status`, error strings, panics.

- `internal/authentik/errors.go` deliberately **drops** the generated client's
  error rather than wrapping it, because that error keeps the raw response body
  reachable via `Body()`. Verify this holds on every path — `mapResponse`,
  `MapResponseError`, `MapError` — and that nothing re-wraps it with `%w`.
- Check that only recognised JSON shapes are extracted from error bodies. An
  echoed request payload must be discarded by construction, not by blocklist.
- `status` is readable by anyone who can read the CR — a wider audience than
  Secrets in the namespace. No credential belongs there.
- Check `logger.Error(err, ...)` call sites in `internal/controller/`.

There is a regression test for this (`TestErrorNeverLeaksSecrets`). Extend it
rather than replacing it.

### 2. Cross-namespace credential access

The sharpest edge in the design.

- A namespaced `AuthentikConnection` must resolve its Secret **only** in its own
  namespace. Verify there is no path around this in
  `internal/controller/connection.go`.
- A `ClusterAuthentikConnection` reads from the namespace named in its spec, so
  creating one is close to a cluster-admin privilege. Verify `allowedNamespaces`
  is evaluated **before** any Secret read, so a disallowed namespace never
  causes a lookup.
- Confirm `SECURITY.md` still describes the behaviour the code implements.

### 3. RBAC

- Read `config/rbac/role.yaml` and the `+kubebuilder:rbac` markers it is
  generated from. Is Secret access broader than needed?
- `charts/authentik-operator/rbac-rules.yaml` is generated from the same source;
  confirm it has not drifted (`make verify` checks this).
- Check the metrics endpoint's authn/authz wiring and the leader-election role.

### 4. Supply chain

- Every third-party GitHub Action pinned to a full commit SHA, not a tag.
- `permissions:` least-privilege per workflow and job.
- No `${{ }}` interpolation of untrusted input (PR titles, branch names) into a
  `run:` block — the classic Actions injection.
- No `pull_request_target` granting fork PRs secrets or write access.
- `go.mod` / `go.sum`: unexpected sources, `replace` directives, pseudo-versions
  where a release exists.
- `Makefile` and `hack/`: pinned tool versions, nothing piping a download to a
  shell, nothing over plain HTTP.
- `Dockerfile`: pinned base, non-root final image, no build secrets in layers.

### 5. Input validation

User-controlled CR fields reaching URLs, API paths or regex engines:

- SSRF via `spec.url` on a connection.
- Path traversal through a slug or name interpolated into an API path.
- ReDoS via `skipPathRegex` on a `ProxyProvider`.
- Anything passed to a shell in `hack/` or `test/`.

### 6. Secrets in the tree

Search everything, including `test/`, `examples/`, `charts/`, `config/`, `docs/`.

`test/authentik/` intentionally contains throwaway local credentials. Judge
whether they are unmistakably marked as such and could not be reused against
anything real — not merely whether they exist.

### 7. Runtime posture

- Container runs non-root, read-only root filesystem, all capabilities dropped,
  seccomp `RuntimeDefault`.
- TLS: `insecureSkipTLSVerify` must never be the default, and a custom CA bundle
  should behave as documented.
- `govulncheck` clean (`make lint` does not run it; CI does).

## Reporting

Report findings ordered by severity, each with:

- **Severity** — critical / high / medium / low / informational
- **Location** — `file:line`
- **What is wrong** — the defect, stated plainly
- **Impact** — the concrete failure or attack path, not a category name
- **Fix** — what to change

Then state briefly which checks came back clean. A named check with a clean
result is part of the report, not an omission.

## Fixing

After reporting, fix confirmed findings. For each:

1. Add or extend a test that fails before the fix. Security fixes without a
   regression test tend to be re-broken.
2. Make the change.
3. Run `make test`, `make lint`, and `make verify`.
4. Commit separately from unrelated work, explaining the impact in the message.

Leave theoretical findings unfixed unless they are cheap; record them instead.
Changing code to satisfy a hypothesis costs more than it returns.
