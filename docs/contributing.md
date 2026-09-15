# Contributing

!!! warning "This project is fully AI generated"

    Human review is the most valuable contribution it can receive. Reviews of
    the [architecture decisions](decisions/0001-connection-scope.md) count just
    as much as code.

`CONTRIBUTING.md` in the repository root is the full document — development
environment, the real `just` recipes, the three test tiers, commit
conventions, and the `just verify` contract. This page is the short version.

## Quick start

```sh
git clone <repository-url>
cd authentik-operator
just build     # generate, format, vet, build bin/manager
just test      # unit + envtest
just           # every recipe, with descriptions
```

Tooling — `controller-gen`, `kustomize`, `setup-envtest`, `golangci-lint`,
`helm-docs`, `crd-ref-docs` — is bootstrapped into `bin/` on first use and pinned in the
justfile. Do not upgrade any of them by hand; bump the `*_VERSION` variable so a
clean checkout and CI stay identical.

## Test tiers

Run the cheapest tier that can catch your mistake.

=== "Unit"

    ```sh
    just test-unit
    ```

    `go test ./internal/... -race -short`. No envtest, no Docker, no cluster.
    Seconds. This is the inner loop.

    Because it passes `-short`, anything needing more than pure Go should guard
    itself with `testing.Short()` and skip.

=== "envtest"

    ```sh
    just test
    ```

    The full suite except `test/e2e`, against a real `kube-apiserver` and
    `etcd`. CRDs apply, admission runs, watches fire — but there is no kubelet,
    so nothing you create becomes a running Pod.

    Reconciler behaviour belongs here: conditions, finalizers, ownership, and
    the cross-namespace `Secret` resolution rules from
    [ADR 0001](decisions/0001-connection-scope.md). Coverage lands in
    `cover.out`.

=== "E2E"

    ```sh
    just authentik-up     # start a local authentik (Docker)
    just test-e2e         # go test ./test/e2e/... -v -timeout 30m
    just authentik-down   # stop it and delete its volumes
    ```

    `just test-e2e` does not start authentik for you, so you can iterate without
    paying the startup cost each time. It does not tear anything down either.

    E2E is for what only a real authentik can prove: API compatibility across
    the [supported versions](operations/supported-versions.md), adoption
    behaviour from [ADR 0002](decisions/0002-adoption-policy.md), and the shape
    of the credentials written back into `Secret`s.

## `just verify` must pass

This repository commits its generated artifacts — CRD YAML, RBAC, `DeepCopy`
methods — and the version matrix must agree with `supported-versions.yaml`.

```sh
just verify   # manifests + generate + verify-versions, then fail if the tree changed
```

If CI says artifacts are out of date:

```sh
just manifests generate
just sync-versions
git add -A && git commit
```

!!! danger "Never hand-edit generated files"

    Anything under `config/crd/bases/`, any `zz_generated.*` file, or the
    version table inside the `<!-- BEGIN SUPPORTED-VERSIONS -->` markers. The
    next generator run discards your change silently.

    Edit the source instead: the Go types, or `supported-versions.yaml`.

## Documentation

This site is [Zensical](https://zensical.org/). Pages live in `docs/`, and the
navigation is defined in `zensical.toml` — **every file listed in `nav` must
exist or the build fails**.

```sh
zensical serve                     # live-reloading preview
zensical build --clean --strict    # what CI runs; warnings are errors
```

!!! warning "Strict mode fails on links that leave `docs/`"

    A relative link such as `../../SECURITY.md` resolves on GitHub and breaks
    the site build, because the target is not a page in the site. Link to
    root-level files by absolute URL, or name them in a code span without
    linking.

Some pages are generated from other files. See
[Reference](reference/index.md#generated-versus-hand-written) for which, and
regenerate with:

```sh
hack/gen-docs.py            # rewrite derived pages
hack/gen-docs.py --check    # what CI runs; writes nothing
```

## Commits

[Conventional Commits](https://www.conventionalcommits.org/):

```
feat(oauth2provider): write generated client credentials to a Secret
fix(connection): resolve cluster connection Secrets only from spec.tokenSecretRef.namespace
docs(security): document the ClusterAuthentikConnection blast radius
```

Types in use: `feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`.
A breaking API change uses `!` before the colon and a `BREAKING CHANGE:` footer.
Summary under about 72 characters, imperative mood, no trailing full stop.

## Before opening a pull request

```sh
just lint
just test
just verify
```

- Explain **why**, not what — the diff shows what.
- Keep it focused. A refactor and a behaviour change in one PR are hard to
  review and harder to revert.
- Include tests at the cheapest tier that can prove the change.
- Update the docs in the same PR: a new kind needs a guide page and a row in the
  status tables; anything touching credentials or RBAC needs `SECURITY.md` and
  [Security](operations/security.md).
- A new condition reason needs a row in [Conditions](reference/conditions.md) —
  that page is hand-written and cannot be generated.
- A design decision with real trade-offs gets an ADR in `docs/decisions/`: next
  number, same structure as the existing two, and a `nav` entry in
  `zensical.toml`.
- Commit generated artifacts.

Rebase on `main` rather than merging it in; keep history linear.

## Reporting issues

Bugs and feature requests go in the issue tracker.

!!! danger "Security vulnerabilities do not"

    Report those privately through GitHub private security advisories. See
    [Security](operations/security.md#reporting-a-vulnerability) and
    `SECURITY.md` in the repository root.

## Licence

By contributing you agree your contributions are licensed under the Apache
License 2.0, the same as the rest of the project.
