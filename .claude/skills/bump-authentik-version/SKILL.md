---
name: bump-authentik-version
description: Update the operator to support a newer authentik release - bump the generated API client, retarget the version gate and CI matrix, reconcile API drift, and verify against a real instance. Use when a new authentik version is out, when asked to support a newer authentik, or when the E2E matrix fails against a newer release.
---

# Bump the supported authentik version

One operator release targets **one** authentik series. Supporting a newer
authentik is therefore not a config edit — it is a retarget of the whole
project, ending in a new versioned release.

Read `README.md` → "Version policy" before starting. If you are trying to
support *two* authentik versions at once, this skill is the wrong tool: see the
Epiq ticket on multi-version support, because the generated client makes that a
design change rather than a bump.

## Why this is more than editing a YAML file

`goauthentik.io/api/v3` is generated from a single authentik release and
**enforces that release's required properties when decoding**. A client built
for 2026.8 marks fields as required that do not exist in 2026.5, so every list
call against the older version fails outright. The client version and the
authentik version move together, or nothing works.

## Steps

### 1. Establish the target

Find the authentik release and the matching client:

```sh
# Real authentik image tags
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:goauthentik/server:pull&service=ghcr.io" | jq -r .token)
curl -s -H "Authorization: Bearer $TOKEN" "https://ghcr.io/v2/goauthentik/server/tags/list?n=2000" \
  | jq -r '.tags[]' | grep -E '^20[0-9]{2}\.[0-9]+\.[0-9]+$' | sort -V | tail -10

# Client versions. The client's version encodes the series: v3.2026080.x is 2026.8
go list -m -versions goauthentik.io/api/v3 | tr ' ' '\n' | tail -15
```

**Verify the tag exists before using it.** Guessing the series has already gone
wrong here once — the real series are `2026.2`, `2026.5`, `2026.8`, not every
even month.

Pick the newest **stable** patch tag, not an `-rc`.

### 2. Bump the client

```sh
go get goauthentik.io/api/v3@<new-client-version>
go mod tidy
go build ./... 2>&1 | head -40
```

Expect breakage. The generated client renames things between releases — an
older one spells the service fields `FlowsApi`, a newer one `FlowsAPI`. Fix the
call sites; do not pin around the problem.

### 3. Reconcile API drift

This is the real work. For each compile error and each field the operator maps:

- Read the model in `$(go env GOMODCACHE)/goauthentik.io/api/v3@<version>/` —
  **never guess a signature or an enum value**.
- Check whether fields the CRDs expose still exist, changed type, or became
  required.
- Check whether new fields are worth exposing. Adding them is optional; breaking
  on removed ones is not.
- Enum values change. Re-extract them:

  ```sh
  M=$(go env GOMODCACHE)/goauthentik.io/api/v3@<version>
  grep -oE '= "[^"]+"' $M/model_<enum>_enum.go | sed 's/= //' | tr '\n' ' '
  ```

  If an enum changed, update the `+kubebuilder:validation:Enum` marker **and**
  remember that values containing `:` need individual quoting, and that a marker
  on a slice field must move to a named element type.

### 4. Retarget the version metadata

`supported-versions.yaml` is the single source of truth. Update `supported`,
`minimum` and `maximum` to the new series, then:

```sh
make sync-versions   # rewrites the README table and bounds
```

Update the Go gate constants in `internal/authentik/version.go` (`MinimumVersion`,
`MaximumVersion`) to match — `TestSupportedVersionConstantsMatchYAML` fails if
they drift.

Add a row to the **compatibility matrix** in `README.md` and
`docs/operations/supported-versions.md` mapping the new operator version to the
new authentik series. Do not delete old rows: users on an older authentik stay
on an older operator release, and the matrix is how they find it.

### 5. Regenerate everything

```sh
make manifests generate
make docs-gen
make verify
```

`make verify` must pass on a clean tree. It also syncs the chart's CRDs and RBAC.

### 6. Verify against a real instance

A compile is not evidence. Run the operator against the new authentik:

```sh
make authentik-up                     # uses the pinned tag from supported-versions.yaml
make test-integration                 # client layer against the live instance
```

Then the full end-to-end suite. If `kind` is not installed,
`go install sigs.k8s.io/kind@latest` into `bin/`:

```sh
./bin/kind create cluster --name ak-bump --wait 120s
./bin/kustomize build config/crd | kubectl --context kind-ak-bump apply -f -
go build -o bin/manager cmd/main.go
./bin/manager --health-probe-bind-address=:18081 --metrics-bind-address=0 &

E2E_ENABLED=true E2E_KUBECONTEXT=kind-ak-bump \
  AUTHENTIK_URL=http://localhost:9000 \
  AUTHENTIK_TOKEN=authentik-operator-e2e-bootstrap-token \
  go test ./test/e2e/... -count=1 -timeout 15m
```

**Run the suite twice.** Fixed-name collisions and stale-status races only show
up on the second run.

Tear down afterwards, and restore the kubecontext you found:

```sh
./bin/kind delete cluster --name ak-bump
make authentik-down
kubectl config use-context <the original context>
```

### 7. Release

Per the version policy, a new authentik target means a **new versioned release**
of the operator and chart, not a patch. Bump `version` and `appVersion` in
`charts/authentik-operator/Chart.yaml`, then tag — the release workflow builds
the multi-arch image, pushes the chart as an OCI artifact and attaches the
manifests.

## Checklist

- [ ] New authentik image tag verified to exist
- [ ] Client bumped, `go mod tidy` clean
- [ ] Every compile error fixed by reading the generated models
- [ ] Enums re-extracted and markers updated
- [ ] `supported-versions.yaml`, the Go gate, and both compatibility matrices agree
- [ ] `make verify` passes on a clean tree
- [ ] `make test` and `make lint` pass
- [ ] E2E green against the new authentik, run **twice**
- [ ] Test environment torn down, kubecontext restored
- [ ] Chart version bumped for the new release

## Failure modes seen before

- **Guessed a version that does not exist.** Always query the tag list.
- **Bumped the matrix without bumping the client**, so CI failed with
  `no value given for required property ...`. They move together.
- **Widened `supported-versions.yaml` to claim a range** the client cannot serve.
  One series per release.
- **Trusted a green E2E run that skipped every scenario.** Check the job summary
  reports executed scenarios, not just a passing exit code.
