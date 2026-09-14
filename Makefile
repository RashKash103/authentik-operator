# authentik-operator

IMG ?= ghcr.io/rashkash103/authentik-operator:latest
ENVTEST_K8S_VERSION = 1.34.0

# Tool versions - pinned so a clean checkout and CI behave identically.
CONTROLLER_TOOLS_VERSION ?= v0.19.0
KUSTOMIZE_VERSION ?= v5.7.1
ENVTEST_VERSION ?= release-0.22
GOLANGCI_LINT_VERSION ?= v2.13.2
HELM_DOCS_VERSION ?= v1.14.2

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
KUSTOMIZE ?= $(LOCALBIN)/kustomize
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
HELM_DOCS ?= $(LOCALBIN)/helm-docs

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CRDs and RBAC.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(MAKE) sync-chart-crds

.PHONY: sync-chart-crds
sync-chart-crds: ## Copy generated CRDs into the Helm chart.
	@rm -f charts/authentik-operator/crds/*.yaml
	@cp config/crd/bases/*.yaml charts/authentik-operator/crds/
	@echo "synced $$(ls config/crd/bases/*.yaml | wc -l) CRDs into the Helm chart"

.PHONY: generate
generate: controller-gen ## Generate DeepCopy methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint with --fix.
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test: manifests generate fmt vet envtest ## Run unit and envtest tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
		go test $$(go list ./... | grep -v /test/e2e) -race -coverprofile=cover.out

.PHONY: test-unit
test-unit: ## Run only the fast unit tests (no envtest, no Docker).
	go test ./internal/... -race -short

.PHONY: sync-versions
sync-versions: ## Regenerate files derived from supported-versions.yaml.
	python3 hack/sync-versions.py

.PHONY: verify-versions
verify-versions: ## Fail if the version matrix has drifted out of sync.
	python3 hack/sync-versions.py --check

.PHONY: verify
verify: manifests generate verify-versions verify-docs ## Fail if generated artifacts are out of date.
	@if ! git diff --quiet --exit-code; then \
		echo "ERROR: generated artifacts are out of date. Run 'make manifests generate' and commit."; \
		git --no-pager diff --stat; \
		exit 1; \
	fi

##@ authentik test instance

.PHONY: authentik-up
authentik-up: ## Start a local authentik for E2E tests.
	./test/authentik/up.sh

.PHONY: authentik-down
authentik-down: ## Stop the local authentik and remove its volumes.
	./test/authentik/down.sh

.PHONY: test-integration
test-integration: ## Run client-layer tests against a running local authentik.
	AUTHENTIK_URL=$${AUTHENTIK_URL:-http://localhost:9000} \
	AUTHENTIK_TOKEN=$${AUTHENTIK_TOKEN:-authentik-operator-e2e-bootstrap-token} \
		go test ./internal/authentik/ -run TestIntegration -v

.PHONY: test-e2e
test-e2e: ## Run the E2E suite against a running local authentik.
	go test ./test/e2e/... -v -timeout 30m

##@ Documentation

ZENSICAL_VERSION ?= 0.0.62

.PHONY: docs
docs: ## Build the documentation site into site/.
	@command -v zensical >/dev/null 2>&1 || { \
		echo "zensical not found. Install it with: pip install zensical==$(ZENSICAL_VERSION)"; exit 1; }
	zensical build --clean --strict

.PHONY: docs-serve
docs-serve: ## Serve the documentation site with live reload.
	zensical serve

.PHONY: docs-gen
docs-gen: ## Regenerate the derived documentation pages.
	python3 hack/gen-docs.py

.PHONY: verify-docs
verify-docs: ## Fail if the derived documentation pages are stale.
	python3 hack/gen-docs.py --check

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build the manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run the manager against the current kubeconfig.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build the container image.
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push the container image.
	docker push $(IMG)

.PHONY: bundle
bundle: manifests kustomize ## Render a single install.yaml.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the cluster.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Remove CRDs from the cluster.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy the operator.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Remove the operator.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found -f -

##@ Tooling

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(LOCALBIN)
	@test -x $(CONTROLLER_GEN) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: kustomize
kustomize: $(LOCALBIN)
	@test -x $(KUSTOMIZE) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: envtest
envtest: $(LOCALBIN)
	@test -x $(ENVTEST) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)

.PHONY: golangci-lint
golangci-lint: $(LOCALBIN)
	@test -x $(GOLANGCI_LINT) || GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: helm-docs
helm-docs: $(LOCALBIN)
	@test -x $(HELM_DOCS) || GOBIN=$(LOCALBIN) go install github.com/norwoodj/helm-docs/cmd/helm-docs@$(HELM_DOCS_VERSION)
