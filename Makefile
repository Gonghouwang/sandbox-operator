# Sandbox Operator Makefile
#
# Provides common development and deployment tasks for the operator.
# Existing scripts under `scripts/` are still used internally by several targets.

# Image name used by docker-build and deploy targets.
IMG ?= sandbox-operator:latest

# Go build settings
GOOS ?= linux
GOARCH ?= amd64

# Manager binary output path
MANAGER_BIN ?= bin/manager

# Silence make output slightly and enable real paths in error messages.
.PHONY: all help build test vet fmt lint docker-build docker-push deploy undeploy clean

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

docker-build: ## Build the container image (IMG defaults to sandbox-operator:latest).
	./scripts/build-image.sh $(IMG)

docker-push: ## Push the container image.
	docker push $(IMG)

##@ Deployment

deploy: ## Deploy the operator to the cluster using raw manifests (set IMG).
	IMAGE=$(IMG) ./scripts/deploy.sh

undeploy: ## Undeploy the operator from the cluster.
	./scripts/undeploy.sh

##@ Cleanup

clean: ## Remove generated build artifacts.
	rm -rf bin/
