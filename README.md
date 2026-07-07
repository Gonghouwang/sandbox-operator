# Sandbox Operator

[![Go Version](https://img.shields.io/badge/Go-1.26-blue.svg)](https://golang.org/)
[![Kubernetes](https://img.shields.io/badge/kubernetes-%3E%3D1.30-blue)](https://kubernetes.io/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

> A Kubernetes operator that manages Kingsoft Cloud Sandbox templates and instances through custom resources.

**English** | [中文](docs/zh-CN/README.md)

---

## Overview

Sandbox Operator provides a Kubernetes-native way to manage [Kingsoft Cloud Sandbox](https://www.ksyun.com/) resources. It exposes three Custom Resource Definitions (CRDs) that mirror the Sandbox OpenAPI concepts:

| Kind | Short Name | Description |
|------|------------|-------------|
| `SandboxTemplate` | `stpl` | Sandbox template describing image, compute, network, storage, logging, and preheat-pool configuration. |
| `Sandbox` | `sbx` | A single sandbox instance. |
| `SandboxClaim` | `sbxc` | A one-shot batch declaration that creates multiple sandbox instances. |

The operator synchronizes resources in both directions:

* **OpenAPI → Kubernetes**: The operator periodically polls the Sandbox OpenAPI and reflects template and instance state into the cluster as CRs.
* **Kubernetes → OpenAPI**: Creating, updating, or deleting a CR invokes the corresponding Sandbox OpenAPI operation. Finalizers ensure cloud resources are cleaned up when the CR is deleted.

## Features

* Three CRDs: `SandboxTemplate`, `Sandbox`, and `SandboxClaim`.
* Bidirectional synchronization between Kubernetes and the Sandbox OpenAPI.
* Mutating and validating admission webhooks.
* Helm Chart and raw Kubernetes manifest deployment options.
* Self-signed or cert-manager-managed webhook TLS certificates.
* Namespace-scoped credentials for multi-tenant use.

## Prerequisites

* A Kubernetes cluster (>= 1.30 recommended).
* `kubectl` configured to access the cluster.
* [Helm 3](https://helm.sh/) (optional, recommended for production).
* A Kingsoft Cloud Sandbox OpenAPI access key pair, account ID, and region.

## Quick Start

### 1. Install the operator

Using Helm:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=my-registry/sandbox-operator \
  --set image.tag=latest
```

Or use the raw manifests:

```bash
make docker-build IMG=my-registry/sandbox-operator:latest
make deploy IMG=my-registry/sandbox-operator:latest
```

### 2. Create OpenAPI credentials in a business namespace

```bash
kubectl create namespace sandbox-demo

kubectl -n sandbox-demo create secret generic sandbox-openapi-credentials \
  --from-literal=accessKeyId='<OPENAPI_ACCESS_KEY_ID>' \
  --from-literal=secretAccessKey='<OPENAPI_SECRET_ACCESS_KEY>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

See the full credential examples in [`config/credentials/credentials.example.yaml`](config/credentials/credentials.example.yaml).

### 3. Create a template and a sandbox

Copy the example YAML from the [CR examples](docs/en/cr-examples.md) guide and apply it to the business namespace:

```bash
kubectl apply -n sandbox-demo -f my-template.yaml
kubectl apply -n sandbox-demo -f my-sandbox.yaml
```

You can also look at the bundled sample in [`config/samples/sandbox_v1alpha1_sample.yaml`](config/samples/sandbox_v1alpha1_sample.yaml) as a starting point.

## Documentation

| Document | Description |
|----------|-------------|
| [Deployment guide](docs/en/deployment.md) | Helm and raw-manifest installation, operator configuration, and credential setup. |
| [Usage guide](docs/en/usage.md) | Detailed workflows for OpenAPI↔Kubernetes sync and CR lifecycle. |
| [CR examples](docs/en/cr-examples.md) | Secret, `SandboxTemplate`, `Sandbox`, inline-template, and `SandboxClaim` YAML examples. |
| [中文文档](docs/zh-CN/README.md) | Simplified Chinese documentation. |

## Operator Configuration

The operator is configured via the `sandbox-operator-config` ConfigMap in the `sandbox-operator-system` namespace. Helm exposes these options through `values.yaml`.

| Name | Default | Description |
|------|---------|-------------|
| `OPENAPI_BASE_URL` | `http://aicp.cn-beijing-6.inner.api.ksyun.com` | Sandbox OpenAPI base URL. |
| `OPENAPI_AUTH_MODE` | `kop-sigv4` | OpenAPI authentication mode. |
| `OPENAPI_SERVICE` | `aicp` | KOP service name. |
| `OPENAPI_VERSION` | `2026-04-01` | OpenAPI version. |
| `DEFAULT_OPENAPI_CREDENTIAL_SECRET` | `sandbox-openapi-credentials` | Default OpenAPI credential Secret name in business namespaces. |
| `POLL_INTERVAL` | `30s` | Polling interval for OpenAPI synchronization. |
| `POLL_PAGE_SIZE` | `100` | Page size for OpenAPI list calls. |
| `MAX_CONCURRENT_NAMESPACES` | `5` | Maximum concurrent namespaces being synchronized. |
| `SYNC_NAMESPACES` | *(empty)* | Comma-separated namespace allowlist; empty means auto-discover namespaces with the default credential Secret. |
| `LEADER_ELECT` | `true` | Enable leader election. |

## Project Layout

```text
api/                 CRD Go type definitions
api/v1alpha1/        current API version
cmd/manager/         Operator manager entrypoint
config/              Kubernetes manifests, CRDs, RBAC, samples, and credentials
config/deploy/       Raw manifest deployment files
config/samples/      Example CRs
charts/              Helm Chart
internal/            Controllers, webhooks, OpenAPI client, field mapping
Makefile             Common development and deployment tasks
scripts/             Build, deploy, and undeploy scripts
docs/                User documentation (English and Chinese)
```

## Development

Build the manager binary:

```bash
make build
```

Run unit tests:

```bash
make test
```

Run code checks:

```bash
make vet
# or
make lint
```

Build the container image:

```bash
make docker-build IMG=sandbox-operator:latest
```

## Contributing

We welcome contributions! Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) for the process for submitting issues, pull requests, and code-review conventions.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Security

If you discover a security vulnerability, please follow the instructions in [`SECURITY.md`](SECURITY.md) to report it responsibly.

## License

Sandbox Operator is licensed under the [Apache License 2.0](LICENSE).
