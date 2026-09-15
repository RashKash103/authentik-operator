# authentik-operator
#
# Run `just` with no arguments for the recipe list.
#
# `just verify` is the contract: it regenerates every derived artifact and fails
# if the tree changed, which is what CI enforces.

set shell := ["bash", "-euo", "pipefail", "-c"]

IMG := env("IMG", "ghcr.io/rashkash103/authentik-operator:latest")
ENVTEST_K8S_VERSION := "1.34.0"

# Tool versions - pinned so a clean checkout and CI behave identically.
CONTROLLER_TOOLS_VERSION := "v0.19.0"
KUSTOMIZE_VERSION := "v5.7.1"
# Pinned to a tag, not the release-0.22 branch: a branch resolves to whatever
# HEAD is today, so the binary gating the whole test suite would be a moving
# target.
ENVTEST_VERSION := "v0.25.1"
GOLANGCI_LINT_VERSION := "v2.13.2"
HELM_DOCS_VERSION := "v1.14.2"
CRD_REF_DOCS_VERSION := "v0.3.0"
ZENSICAL_VERSION := "0.0.62"

LOCALBIN := justfile_directory() / "bin"
CONTROLLER_GEN := LOCALBIN / "controller-gen"
KUSTOMIZE := LOCALBIN / "kustomize"
ENVTEST := LOCALBIN / "setup-envtest"
GOLANGCI_LINT := LOCALBIN / "golangci-lint"
HELM_DOCS := LOCALBIN / "helm-docs"
CRD_REF_DOCS := LOCALBIN / "crd-ref-docs"

# Show the recipe list.
_default:
    @just --list --unsorted

# ---------------------------------------------------------------- development

# Generate CRDs and RBAC.
[group('development')]
manifests: controller-gen
    {{ CONTROLLER_GEN }} rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
    @just sync-chart-crds

# Copy generated CRDs and RBAC into the Helm chart.
[group('development')]
sync-chart-crds:
    @rm -f charts/authentik-operator/crds/*.yaml
    @cp config/crd/bases/*.yaml charts/authentik-operator/crds/
    @echo "synced $(ls config/crd/bases/*.yaml | wc -l) CRDs into the Helm chart"
    @python3 hack/sync-chart-rbac.py

# Generate DeepCopy methods.
[group('development')]
generate: controller-gen
    {{ CONTROLLER_GEN }} object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Run go fmt.
[group('development')]
fmt:
    go fmt ./...

# Run go vet.
[group('development')]
vet:
    go vet ./...

# Run golangci-lint.
[group('development')]
lint: golangci-lint
    {{ GOLANGCI_LINT }} run

# Run golangci-lint with --fix.
[group('development')]
lint-fix: golangci-lint
    {{ GOLANGCI_LINT }} run --fix

# Run unit and envtest tests.
[group('development')]
test: manifests generate fmt vet envtest
    KUBEBUILDER_ASSETS="$({{ ENVTEST }} use {{ ENVTEST_K8S_VERSION }} --bin-dir {{ LOCALBIN }} -p path)" \
        go test $(go list ./... | grep -v /test/e2e) -race -coverprofile=cover.out

# Run only the fast unit tests (no envtest, no Docker).
[group('development')]
test-unit:
    go test ./internal/... -race -short

# Regenerate files derived from supported-versions.yaml.
[group('development')]
sync-versions:
    python3 hack/sync-versions.py

# Propagate the chart version into every install command.
[group('development')]
sync-release-version:
    python3 hack/sync-release-version.py

# Extract JSON Schemas for editor validation of manifests.
[group('development')]
schemas: manifests
    python3 hack/gen-schemas.py

# Sanity-check the generated CRD schemas.
[group('development')]
check-crds: manifests
    python3 hack/check-crds.py

# --------------------------------------------------------------------- verify

# Fail if the version matrix has drifted out of sync.
[group('verify')]
verify-versions:
    python3 hack/sync-versions.py --check

# Fail if a documented install command quotes the wrong version.
[group('verify')]
verify-release-version:
    python3 hack/sync-release-version.py --check

# Fail if the chart's RBAC has drifted from the generated role.
[group('verify')]
verify-chart-rbac:
    python3 hack/sync-chart-rbac.py --check

# Fail if the JSON Schemas are stale.
[group('verify')]
verify-schemas:
    python3 hack/gen-schemas.py --check

# Fail if the derived documentation pages are stale.
[group('verify')]
verify-docs:
    python3 hack/gen-docs.py --check

# Fail if the API reference is stale.
[group('verify')]
verify-api-docs: crd-ref-docs
    {{ CRD_REF_DOCS }} --source-path=./api --config=hack/crd-ref-docs.yaml \
        --renderer=markdown --output-path={{ LOCALBIN }}/api-reference.md
    python3 hack/gen-api-docs.py {{ LOCALBIN }}/api-reference.md --check

# Fail if the Helm values reference is stale.
[group('verify')]
verify-helm-docs: helm-docs
    python3 hack/gen-helm-docs.py --check

# Fail if any generated artifact is out of date. This is the contract.
[group('verify')]
verify: manifests generate verify-versions verify-release-version verify-docs verify-api-docs verify-helm-docs verify-chart-rbac verify-schemas check-crds
    #!/usr/bin/env bash
    set -euo pipefail
    if ! git diff --quiet --exit-code; then
        echo "ERROR: generated artifacts are out of date. Run 'just manifests generate' and commit."
        git --no-pager diff --stat
        exit 1
    fi

# ------------------------------------------------------- authentik test instance

# Start a local authentik for E2E tests.
[group('authentik')]
authentik-up:
    ./test/authentik/up.sh

# Stop the local authentik and remove its volumes.
[group('authentik')]
authentik-down:
    ./test/authentik/down.sh

# Run client-layer tests against a running local authentik.
[group('authentik')]
test-integration:
    AUTHENTIK_URL="${AUTHENTIK_URL:-http://localhost:9000}" \
    AUTHENTIK_TOKEN="${AUTHENTIK_TOKEN:-authentik-operator-e2e-bootstrap-token}" \
        go test ./internal/authentik/ -run TestIntegration -v

# Run the E2E suite against a running local authentik.
[group('authentik')]
test-e2e:
    go test ./test/e2e/... -v -timeout 30m

# -------------------------------------------------------------- documentation

# Build the documentation site into site/.
[group('docs')]
docs:
    @command -v zensical >/dev/null 2>&1 || { \
        echo "zensical not found. Install it with: pip install zensical=={{ ZENSICAL_VERSION }}"; exit 1; }
    zensical build --clean --strict

# Serve the documentation site with live reload.
[group('docs')]
docs-serve:
    zensical serve

# Regenerate the API reference from the Go types.
[group('docs')]
docs-api: crd-ref-docs
    {{ CRD_REF_DOCS }} --source-path=./api --config=hack/crd-ref-docs.yaml \
        --renderer=markdown --output-path={{ LOCALBIN }}/api-reference.md
    python3 hack/gen-api-docs.py {{ LOCALBIN }}/api-reference.md

# Regenerate the Helm values reference.
[group('docs')]
docs-helm: helm-docs
    python3 hack/gen-helm-docs.py

# Regenerate every derived documentation page.
[group('docs')]
docs-gen: docs-api docs-helm
    python3 hack/gen-docs.py

# ---------------------------------------------------------------------- build

# Build the manager binary.
[group('build')]
build: manifests generate fmt vet
    go build -o bin/manager cmd/main.go

# Run the manager against the current kubeconfig.
[group('build')]
run: manifests generate fmt vet
    go run ./cmd/main.go

# Build the container image.
[group('build')]
docker-build:
    docker build -t {{ IMG }} .

# Push the container image.
[group('build')]
docker-push:
    docker push {{ IMG }}

# Render a single install.yaml.
[group('build')]
bundle: manifests kustomize
    mkdir -p dist
    cd config/manager && {{ KUSTOMIZE }} edit set image controller={{ IMG }}
    {{ KUSTOMIZE }} build config/default > dist/install.yaml

# ----------------------------------------------------------------- deployment

# Install CRDs into the cluster.
[group('deployment')]
install: manifests kustomize
    {{ KUSTOMIZE }} build config/crd | kubectl apply -f -

# Remove CRDs from the cluster.
[group('deployment')]
uninstall: manifests kustomize
    {{ KUSTOMIZE }} build config/crd | kubectl delete --ignore-not-found -f -

# Deploy the operator.
[group('deployment')]
deploy: manifests kustomize
    cd config/manager && {{ KUSTOMIZE }} edit set image controller={{ IMG }}
    {{ KUSTOMIZE }} build config/default | kubectl apply -f -

# Remove the operator.
[group('deployment')]
undeploy: kustomize
    {{ KUSTOMIZE }} build config/default | kubectl delete --ignore-not-found -f -

# -------------------------------------------------------------------- tooling
#
# Each tool is installed into bin/ at its pinned version if it is not already
# there. The `test -x` guard is what makes these cheap to depend on.

_localbin:
    @mkdir -p {{ LOCALBIN }}

[group('tooling')]
controller-gen: _localbin
    @test -x {{ CONTROLLER_GEN }} || GOBIN={{ LOCALBIN }} go install sigs.k8s.io/controller-tools/cmd/controller-gen@{{ CONTROLLER_TOOLS_VERSION }}

[group('tooling')]
kustomize: _localbin
    @test -x {{ KUSTOMIZE }} || GOBIN={{ LOCALBIN }} go install sigs.k8s.io/kustomize/kustomize/v5@{{ KUSTOMIZE_VERSION }}

[group('tooling')]
envtest: _localbin
    @test -x {{ ENVTEST }} || GOBIN={{ LOCALBIN }} go install sigs.k8s.io/controller-runtime/tools/setup-envtest@{{ ENVTEST_VERSION }}

[group('tooling')]
golangci-lint: _localbin
    @test -x {{ GOLANGCI_LINT }} || GOBIN={{ LOCALBIN }} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ GOLANGCI_LINT_VERSION }}

[group('tooling')]
helm-docs: _localbin
    @test -x {{ HELM_DOCS }} || GOBIN={{ LOCALBIN }} go install github.com/norwoodj/helm-docs/cmd/helm-docs@{{ HELM_DOCS_VERSION }}

[group('tooling')]
crd-ref-docs: _localbin
    @test -x {{ CRD_REF_DOCS }} || GOBIN={{ LOCALBIN }} go install github.com/elastic/crd-ref-docs@{{ CRD_REF_DOCS_VERSION }}
