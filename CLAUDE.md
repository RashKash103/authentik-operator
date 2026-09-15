# authentik-operator

A Kubernetes operator that manages objects **inside an existing authentik
instance** over its REST API. It does not deploy authentik.

## Orientation

| Path | What lives there |
| --- | --- |
| `api/v1alpha1/` | CRD Go types. Godoc comments become the published schema. |
| `internal/authentik/` | Wrapper over the generated authentik client: reference resolvers, typed errors, version gate. |
| `internal/controller/` | Reconcilers, plus the shared reconcile engine in `managed.go`. |
| `config/` | kustomize: CRDs, RBAC, manager Deployment. Mostly generated. |
| `charts/authentik-operator/` | Helm chart. CRDs and RBAC inside it are generated. |
| `test/e2e/`, `test/utils/` | End-to-end suite and its harness. |
| `test/authentik/` | Docker Compose authentik for local and CI testing. |
| `hack/` | Generators and consistency checks wired into `just verify`. |
| `docs/` | Zensical documentation site sources. |

## Commands

```sh
just test          # unit + envtest. Needs no Docker.
just test-unit     # fast subset, no envtest binaries
just verify        # regenerate everything and fail if the tree is dirty
just lint          # golangci-lint
just authentik-up  # local authentik on :9000
just test-e2e      # E2E suite (see the opt-in below)
just docs          # build the docs site
```

`just verify` is the contract. It regenerates CRDs, deepcopy, RBAC, the chart's
copies of both, the README version table, and the docs, then fails if anything
changed. If CI says "generated artifacts are current" failed, run it and commit.

## Hard-won rules

These each cost real time. Read before touching the relevant area.

### CRD markers

- **An enum marker on a slice field constrains the array, not its items**, which
  rejects every non-empty list. Put the marker on a named element type:

  ```go
  // +kubebuilder:validation:Enum=a;b
  type GrantType string
  GrantTypes []GrantType   // not: marker on []string
  ```

- **Enum values containing `:` must be quoted individually**, or the marker
  parser reads them as argument separators.
- **Never put implementation notes in a field's godoc.** It becomes the CRD
  description and ships to users via `kubectl explain`.
- **Avoid straight single quotes in CEL rules and in comments near them.**
  `gofmt` has rewritten `''` in an adjacent comment into typographic quotes,
  silently corrupting a rule. Prefer `size(self.x) > 0` over `self.x != ''`.
- Every CEL rule and validation needs an envtest case in
  `internal/controller/crdvalidation_test.go` asserting **both** acceptance and
  rejection. A rule that never fires looks identical to one that works if you
  only test the rejecting side.

`hack/check-crds.py` catches the first of these; the rest are on you.

### The authentik client

- `goauthentik.io/api/v3` is generated from **one** authentik release and
  enforces that release's required properties when decoding. This is why the
  operator supports one authentik series per release. See the version policy in
  `README.md` before widening anything.
- The generated client keeps the raw response body reachable on its error type.
  `internal/authentik/errors.go` therefore **drops** that error rather than
  wrapping it, and rebuilds a typed error from the status plus recognised JSON
  shapes. Do not `%w`-wrap a generated client error.
- authentik objects are keyed by **name/slug, not by Kubernetes UID**. Adoption
  conflicts are routine, not exceptional.

### Controllers

- Build on `Sync`/`Finalize` in `internal/controller/managed.go`. A resource kind
  supplies a `RemoteAdapter`; it does not reimplement lifecycle logic.
- Persist the finalizer **before** creating anything remotely. The other order
  leaks authentik objects when the process dies mid-create.
- `status.remoteID` beats the name on lookup: either side may be renamed, and
  preferring the ID is what stops a rename becoming a duplicate.
- `Ready` and `Synced` mean different things. Reaching the API and being in sync
  are separate questions; collapsing them hides which one failed.
- Distinguish "not yet" from "never" via `ResultFor`. A missing reference
  requeues; an ambiguous reference or rejected spec must not spin.
- Credentials never go in `status` or logs. `status` is readable by a wider
  audience than Secrets in the namespace.

### Tests

- **The E2E suite is opt-in** (`E2E_ENABLED=true` plus an explicitly named
  `E2E_KUBECONTEXT`). It creates namespaces and CRDs, and a developer's current
  context is often a real cluster. Never make it default to the ambient context.
- **Name authentik objects uniquely per run** with `utils.UniqueName`. Namespaces
  are per-test; the authentik instance outlives the run, so fixed names collide
  on the second run.
- **Waiting on `Ready=True` after editing a spec is a race** — it is already true
  from the previous reconcile. Use `utils.WaitForGenerationSynced`, which
  compares the condition's `observedGeneration` against `metadata.generation`.
- Green is not the same as tested. The E2E job once passed while every scenario
  skipped. If you add a suite, make it report what actually ran.

### Helm chart

- **Everything in `charts/authentik-operator/ci/` is installed by
  chart-testing**, so each file must be installable with no setup beyond the
  namespace `ct` creates. Render-only configurations go in `test-values/`. See
  `charts/authentik-operator/ci/README.md`.
- The chart's CRDs and ClusterRole rules are **generated** — edit the Go markers,
  not the chart.
- Helm installs `crds/` once and never upgrades it. Any doc describing an upgrade
  must say so.

## Conventions

- Conventional Commits. Commit messages explain **why**, not what; the diff shows
  what. Reference Epiq ticket refs where one applies.
- Work is tracked in Epiq (`.epiq/`, state on the `__epiq_state__` branch). The
  CLI is TUI-only; drive the bundled `epiq-mcp` MCP server for scripted access,
  with `EPIQ_USER_NAME` set so writes are attributed correctly.
- Prefer verifying over assuming. Most of the real bugs in this repository were
  found by running something — installing the chart, running the suite twice,
  reading operator logs — not by reading the code.
