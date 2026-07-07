# Deployment Guide

This guide describes how to deploy Sandbox Operator.

## 1. What Is Deployed

Sandbox Operator installs the following Kubernetes resources:

* `Namespace`: `sandbox-operator-system` by default.
* CRDs: `SandboxTemplate`, `Sandbox`, `SandboxClaim`.
* RBAC: ServiceAccount, ClusterRole, and ClusterRoleBinding used by the operator.
* ConfigMap: operator configuration.
* Deployment: operator manager.
* Service: webhook service.
* `MutatingWebhookConfiguration`: injects the platform ID when creating CRs.
* `ValidatingWebhookConfiguration`: validates CR updates and deletions.
* Webhook TLS Secret: serving certificate for the webhook service.

## 2. Deploy with Helm (Recommended)

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=sandbox-operator \
  --set image.tag=latest
```

If the image is in a private registry, configure the repository and pull secret:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository='<IMAGE_REPOSITORY>' \
  --set image.tag='<IMAGE_TAG>' \
  --set imagePullSecrets[0].name='<IMAGE_PULL_SECRET>'
```

By default, the chart generates a self-signed webhook certificate using Helm templates.

If cert-manager is already installed, you can let cert-manager generate the certificate instead:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```

## 3. Deploy with Raw Manifests

Build the image:

```bash
make docker-build IMG=sandbox-operator:latest
```

Deploy:

```bash
make deploy IMG=sandbox-operator:latest
```

The script performs the following steps:

1. Applies the CRD, RBAC, ConfigMap, Deployment, and webhook configuration files under `config/deploy`.
2. Generates a self-signed CA and serving certificate with `openssl`.
3. Creates the `sandbox-operator-webhook-server-cert` Secret.
4. Patches the CA bundle into the webhook configurations.
5. Waits for the operator Deployment to become ready.

Uninstall:

```bash
make undeploy
```

## 4. Operator Configuration

Configuration is read from the `sandbox-operator-config` ConfigMap. When deploying with Helm, these values can be overridden in `values.yaml`.

| Setting | Default | Description |
| --- | --- | --- |
| `OPENAPI_BASE_URL` | `http://aicp.cn-beijing-6.inner.api.ksyun.com` | Sandbox OpenAPI base URL. |
| `OPENAPI_AUTH_MODE` | `kop-sigv4` | OpenAPI signature mode. |
| `OPENAPI_SERVICE` | `aicp` | KOP service. |
| `OPENAPI_VERSION` | `2026-04-01` | OpenAPI version. |
| `DEFAULT_OPENAPI_CREDENTIAL_SECRET` | `sandbox-openapi-credentials` | Default OpenAPI credential Secret name in business namespaces. |
| `POLL_INTERVAL` | `30s` | OpenAPI polling interval. |
| `POLL_PAGE_SIZE` | `100` | Page size for OpenAPI list calls. |
| `MAX_CONCURRENT_NAMESPACES` | `5` | Number of namespaces synchronized concurrently. |
| `SYNC_NAMESPACES` | *(empty)* | Namespace allowlist; if empty, namespaces containing the default OpenAPI credential Secret are discovered automatically. |
| `LEADER_ELECT` | `true` | Enable leader election. |

## 5. Create Business Namespace Credentials

The operator synchronizes only business namespaces that contain a valid OpenAPI credential Secret.

```bash
kubectl create namespace sandbox-demo

kubectl -n sandbox-demo create secret generic sandbox-openapi-credentials \
  --from-literal=accessKeyId='<OPENAPI_ACCESS_KEY_ID>' \
  --from-literal=secretAccessKey='<OPENAPI_SECRET_ACCESS_KEY>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

Runtime credentials (for example KS3, KPFS, Klog, and image registry credentials) must also be placed in the same namespace where the business CRs live.

Full Secret examples are available in [CR examples](cr-examples.md) and [`config/credentials/credentials.example.yaml`](../../config/credentials/credentials.example.yaml).

## 6. Verify the Deployment

```bash
kubectl get pods -n sandbox-operator-system
kubectl get mutatingwebhookconfiguration sandbox-operator-mutating-webhook
kubectl get validatingwebhookconfiguration sandbox-operator-validating-webhook
kubectl get crd sandboxtemplates.sandbox.kce.ksyun.com
kubectl get crd sandboxes.sandbox.kce.ksyun.com
kubectl get crd sandboxclaims.sandbox.kce.ksyun.com
```

View logs:

```bash
kubectl logs -n sandbox-operator-system deploy/sandbox-operator
```
