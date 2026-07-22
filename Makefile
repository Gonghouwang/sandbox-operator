# Sandbox Operator Makefile
#
# Provides common development and deployment tasks for the operator.
# Existing scripts under `scripts/` are still used internally by several targets.

# Public image used by deploy targets. Override IMG to deploy a self-built image.
IMG ?= hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707
# Operator namespace used by the raw-manifest deploy and undeploy targets.
NAMESPACE ?= sandbox-operator-system
# Optional image pull Secret used by the raw-manifest deploy target.
IMAGE_PULL_SECRET ?=
# Sandbox OpenAPI endpoint used by the raw-manifest deploy target.
OPENAPI_BASE_URL ?= http://aicp.cn-beijing-6.api.ksyun.com

# Go build settings
GOOS ?= linux
GOARCH ?= amd64

# Manager binary output path
MANAGER_BIN ?= bin/manager

# Silence make output slightly and enable real paths in error messages.
.PHONY: all help build test vet fmt lint docker-build docker-push deploy undeploy purge-crds clean

all: build

##@ General

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

build: ## Build the manager binary.
	go build -o $(MANAGER_BIN) ./cmd/manager

test: ## Run unit tests.
	go test ./...

vet: ## Run go vet against code.
	go vet ./...

fmt: ## Run gofmt against code.
	gofmt -w .

lint: fmt vet ## Run formatting and vet checks.

##@ Build

docker-build: ## Build the container image (override IMG to choose the target image).
	./scripts/build-image.sh $(IMG)

docker-push: ## Push the container image.
	docker push $(IMG)

##@ Deployment

deploy: ## Deploy the operator to the cluster using raw manifests.
	IMAGE=$(IMG) IMAGE_PULL_SECRET=$(IMAGE_PULL_SECRET) NAMESPACE=$(NAMESPACE) OPENAPI_BASE_URL=$(OPENAPI_BASE_URL) ./scripts/deploy.sh

undeploy: ## Undeploy the operator from the cluster.
	NAMESPACE=$(NAMESPACE) ./scripts/undeploy.sh

purge-crds: ## Delete the operator CRDs after all Sandbox CRs have been removed.
	./scripts/purge-crds.sh

##@ Cleanup

clean: ## Remove generated build artifacts.
	rm -rf bin/
