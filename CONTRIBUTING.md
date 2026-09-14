# Contributing

> [!WARNING]
> **This project is fully AI generated.** Human review is the most valuable
> contribution it can receive. Reviews of the design documents in
> [`docs/decisions/`](docs/decisions/) count just as much as code.

Thanks for taking a look. This document covers the development environment, the
Makefile targets that actually exist, how to run each tier of tests, and the
`make verify` contract that CI enforces.

## Prerequisites

| Tool       | Needed for                                        | Notes                                                     |
| ---------- | ------------------------------------------------- | --------------------------------------------------------- |
| Go         | Everything                                        | Version per [`go.mod`](go.mod).                            |
| `make`     | Everything                                        | GNU make. The Makefile uses `bash -o pipefail`.            |
| Docker     | E2E tests, image builds                           | Or a compatible runtime for `docker build`/`docker push`.  |
| `kubectl`  | `make install`, `make deploy`                     | Pointed at whatever cluster you want to target.            |
| A cluster  | envtest is self-contained; `deploy` is not        | kind, k3d, minikube — anything.                            |

Everything else is bootstrapped into `./bin` on first use and pinned in the
Makefile: `controller-gen`, `kustomize`, `setup-envtest`, `golangci-lint` and
`helm-docs`. You do not install those by hand, and you should not upgrade them
by hand either — bump the `*_VERSION` variable at the top of the Makefile so a
clean checkout and CI stay identical.

## Getting started

```sh
git clone <repository-url>
cd authentik-operator
make build            # generates, formats, vets, and builds bin/manager
make test             # unit + envtest
```

`make help` prints every target with its description; it is generated from the
Makefile itself, so it never goes stale.

### Running the manager locally

`make run` runs the manager against whatever your current kubeconfig points at,
outside the cluster. Install the CRDs first:

```sh
make install
make run
```

`make run` passes no flags of its own. To run the manager with flags, build it
and run the binary directly:

```sh
make build
./bin/manager --leader-elect=false --metrics-bind-address=0 --zap-devel=true
```

The flags the manager accepts are `--metrics-bind-address`,
`--health-probe-bind-address`, `--leader-elect`, `--metrics-secure`,
`--enable-http2` and `--watch-namespaces`, plus the controller-runtime zap
logging flags. The README has the
[full table with defaults](README.md#manager-flags). When iterating locally,
`--watch-namespaces` keeps the cache small and `--metrics-bind-address=0`
avoids a port conflict.

### Deploying to a cluster

```sh
make docker-build IMG=<your-registry>/authentik-operator:dev
make docker-push  IMG=<your-registry>/authentik-operator:dev
make deploy       IMG=<your-registry>/authentik-operator:dev
```

`make undeploy` removes the controller-manager; `make uninstall` removes the
CRDs. `make bundle` renders the whole thing into `dist/install.yaml` without
applying it — useful for reviewing exactly what `deploy` would do.

> [!NOTE]
> `make deploy` and `make bundle` both run `kustomize edit set image` inside
> `config/manager`, which **modifies `config/manager/kustomization.yaml` in your
> working tree**. Check `git status` before committing after running either.

## Makefile targets

These are the real targets. If you find one referenced in documentation that is
not in this list, the documentation is wrong — please fix it or open an issue.

### Development

| Target          | What it does                                                        |
| --------------- | ------------------------------------------------------------------- |
| `make help`     | Print every target with its description.                             |
| `make manifests`| Regenerate CRDs and RBAC into `config/`.                             |
| `make generate` | Regenerate `DeepCopy` methods.                                       |
| `make fmt`      | `go fmt ./...`                                                       |
| `make vet`      | `go vet ./...`                                                       |
| `make lint`     | Run `golangci-lint`.                                                 |
| `make lint-fix` | Run `golangci-lint --fix`.                                           |
| `make test`     | Unit + envtest tests with `-race` and coverage into `cover.out`.     |
| `make test-unit`| Fast unit tests only — no envtest, no Docker.                        |
| `make verify`   | Fail if generated artifacts are out of date.                         |

### authentik test instance

| Target                | What it does                                                 |
| --------------------- | ------------------------------------------------------------ |
| `make authentik-up`   | Start a local authentik for E2E tests.                        |
| `make authentik-down` | Stop the local authentik and remove its volumes.              |
| `make test-e2e`       | Run the E2E suite against a running local authentik.          |

### Build

| Target              | What it does                                    |
| ------------------- | ----------------------------------------------- |
| `make build`        | Build `bin/manager`.                             |
| `make run`          | Run the manager against the current kubeconfig.  |
| `make docker-build` | Build the container image as `$(IMG)`.           |
| `make docker-push`  | Push `$(IMG)`.                                   |
| `make bundle`       | Render `dist/install.yaml`.                      |

### Deployment

| Target           | What it does                            |
| ---------------- | --------------------------------------- |
| `make install`   | Apply the CRDs to the current cluster.   |
| `make uninstall` | Delete the CRDs.                         |
| `make deploy`    | Apply CRDs + controller-manager.         |
| `make undeploy`  | Delete the controller-manager.           |

### Variables

| Variable              | Default                                      | Purpose                             |
| --------------------- | -------------------------------------------- | ----------------------------------- |
| `IMG`                 | `ghcr.io/rashkash103/authentik-operator:latest`   | Image built, pushed and deployed.    |
| `ENVTEST_K8S_VERSION` | `1.34.0`                                     | Control-plane version envtest uses.  |

## Tests

There are three tiers, and they cost very different amounts to run. Run the
cheapest one that can catch your mistake.

### Unit tests

```sh
make test-unit
```

Runs `go test ./internal/... -race -short`. No envtest binaries, no Docker, no
cluster. Seconds. This is the inner loop — use it constantly.

Because it passes `-short`, any test that needs more than pure Go should guard
itself with `testing.Short()` and skip.

### envtest

```sh
make test
```

Runs the full suite except `test/e2e`, against a real `kube-apiserver` and
`etcd` that `setup-envtest` downloads into `./bin` (Kubernetes `1.34.0` by
default). There is an API server, so CRDs apply, admission runs and watches
fire — but there is **no kubelet and no controller-manager**, so nothing you
create ever becomes a running Pod and built-in controllers do not act.

This target also runs `manifests`, `generate`, `fmt` and `vet` first, so it is
the closest single command to what CI checks. It writes coverage to `cover.out`:

```sh
go tool cover -html=cover.out
```

Reconciler behaviour — conditions, finalizers, ownership, the cross-namespace
`Secret` resolution rules from [ADR 0001](docs/decisions/0001-connection-scope.md)
— belongs here, with a faked authentik API.

### E2E

E2E runs against a **real authentik**, started locally.

```sh
make authentik-up     # start local authentik (Docker)
make test-e2e         # go test ./test/e2e/... -v -timeout 30m
make authentik-down   # stop it and delete its volumes
```

`make test-e2e` does not start authentik for you — it assumes one is already
running, so that you can iterate on the suite without paying the startup cost
each time. It also does not tear anything down; run `make authentik-down` when
you are finished, since it removes the volumes and gives you a clean instance
next time.

The suite has a 30-minute timeout because authentik takes a while to become
ready and because outpost reconciliation is slow. If you are debugging one test,
narrow it with `-run`:

```sh
go test ./test/e2e/... -v -timeout 30m -run TestOAuth2Provider
```

E2E is for things only a real authentik can prove: API compatibility across the
[supported versions](README.md#supported-authentik-versions), adoption
behaviour from [ADR 0002](docs/decisions/0002-adoption-policy.md), and the shape
of the credentials written back into `Secret`s.

## `make verify` must pass

This repository commits its generated artifacts — CRD YAML, RBAC, `DeepCopy`
methods. `make verify` runs `manifests` and `generate` and then fails if the
working tree changed, which means **generated output must be committed along
with the source change that produced it**.

If CI tells you artifacts are out of date:

```sh
make manifests generate
git add -A
git commit --amend --no-edit   # or a new commit
```

Anything that touches `api/` will move generated files. That is expected; commit
them. Do not hand-edit files under `config/crd/bases/` or any `zz_generated.*`
file — the next `make manifests` will silently discard your changes.

## Supported authentik versions

[`supported-versions.yaml`](supported-versions.yaml) is the single source of
truth for which authentik series this operator supports. It feeds the CI E2E
matrix, the table in the README, and the runtime compatibility gate. Changing
the supported set means changing that file **and** the README table, and CI
verifies the two agree.

To change the supported set, edit `supported-versions.yaml` and then run:

```sh
make sync-versions   # rewrites the README table from that file
make verify-versions # fails if anything is still out of step
```

`make verify` runs the check, so a stale README cannot reach `main`. The Go
version gate in `internal/authentik/version.go` is checked but never rewritten:
a regex rewrite of the gate would be more dangerous than useful, so if its
constants disagree the check tells you and you edit them by hand.

## Commit conventions

Commits follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<optional scope>): <summary in the imperative mood>

<optional body explaining why, not what>

<optional footers>
```

Types in use:

| Type       | For                                                        |
| ---------- | ---------------------------------------------------------- |
| `feat`     | A new capability.                                           |
| `fix`      | A bug fix.                                                  |
| `docs`     | Documentation only.                                         |
| `test`     | Tests only.                                                 |
| `refactor` | A change with no behavioural difference.                    |
| `build`    | Build system, Dockerfile, dependencies.                     |
| `ci`       | CI configuration.                                           |
| `chore`    | Everything else.                                            |

Useful scopes: an API kind (`oauth2provider`), an area (`api`, `rbac`, `helm`,
`e2e`).

A breaking API change uses `!` before the colon and a `BREAKING CHANGE:` footer
explaining the migration. While the API is `v1alpha1` breaking changes are
allowed, but they still have to be announced.

Examples:

```
feat(oauth2provider): write generated client credentials to a Secret
fix(connection): resolve cluster connection Secrets only from spec.tokenSecretRef.namespace
docs(security): document the ClusterAuthentikConnection blast radius
feat(api)!: rename spec.target to spec.connectionRef

BREAKING CHANGE: spec.target is removed. Manifests must use spec.connectionRef.
```

Keep the summary under about 72 characters, lower case after the colon, no
trailing full stop, imperative mood ("add", not "added" or "adds").

## Pull requests

Before opening one:

```sh
make lint
make test
make verify
```

In the PR itself:

- Explain **why**, not just what. The diff shows what.
- Keep it focused. A refactor and a behaviour change in one PR are hard to
  review and harder to revert.
- Include tests at the cheapest tier that can prove the change.
- Update the docs in the same PR: the README table for a new kind, `SECURITY.md`
  for anything touching credentials or RBAC.
- If the change reflects a design decision with real trade-offs, add an ADR to
  `docs/decisions/` — next number, same structure as the existing two.
- Commit generated artifacts. See [`make verify` must pass](#make-verify-must-pass).

Rebase on `main` rather than merging it in; keep history linear.

## Reporting issues

Bugs and feature requests go in the issue tracker.

**Security vulnerabilities do not.** See [`SECURITY.md`](SECURITY.md) — report
those privately through GitHub security advisories.

## Licence

By contributing you agree that your contributions are licensed under the
Apache License 2.0, the same as the rest of the project. See [LICENSE](LICENSE).

## Documentation site

The site is built with [Zensical](https://zensical.org/) from `docs/` and
published to GitHub Pages by the `Documentation` workflow on every push to
`main`.

```sh
pip install zensical==0.0.62
make docs        # build into site/ with --strict
make docs-serve  # live reload
make verify-docs # fail if derived pages are stale
```

GitHub Pages has to be enabled **once per repository**, with the source set to
"GitHub Actions". The workflow cannot do this itself: creating a Pages site
needs admin rights that `GITHUB_TOKEN` is not granted, so `configure-pages`
fails with *Resource not accessible by integration*. Enable it in
**Settings → Pages**, or with:

```sh
gh api --method POST repos/OWNER/REPO/pages -f build_type=workflow
```
