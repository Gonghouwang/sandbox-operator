# Raw Kubernetes Deployment Resources

This page describes the raw Kubernetes manifests used when deploying without Helm.

The manifest directory is `config/deploy/`:

| File | Description |
| --- | --- |
| `00-namespace.yaml` | Operator namespace. |
| `01-crd.yaml` | CRDs for `SandboxTemplate`, `Sandbox`, and `SandboxClaim`. |
| `02-rbac.yaml` | ServiceAccount, ClusterRole, and ClusterRoleBinding. |
| `03-config.yaml` | Operator configuration ConfigMap. |
| `04-manager.yaml` | Operator Deployment. |
| `05-webhook.yaml` | Webhook Service, MutatingWebhookConfiguration, and ValidatingWebhookConfiguration. |

## Deploy

From the repository root, run:

```bash
make deploy
```

The command applies these resources in the required order, creates the webhook TLS Secret, and patches the webhook CA bundle. It uses the public image `hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707`; no image-pull credential is required. To use your own image, override it at deployment time:

```bash
make deploy IMG=my-registry.example.com/sandbox-operator:v0.1.0
```

For a private registry, create an image pull Secret in `sandbox-operator-system` and add `IMAGE_PULL_SECRET=<SECRET_NAME>`:

```bash
make deploy IMG=my-registry.example.com/sandbox-operator:v0.1.0 IMAGE_PULL_SECRET=sandbox-operator-image-pull
```

Uninstall:

```bash
make undeploy
```

## Webhook Certificates

Raw manifest deployment does not require cert-manager. `scripts/deploy.sh`:

1. Generates a self-signed CA and serving certificate with `openssl`.
2. Writes the serving certificate into the `sandbox-operator-webhook-server-cert` Secret.
3. Patches the CA bundle into MutatingWebhookConfiguration and ValidatingWebhookConfiguration.

If you manually replace the webhook certificate, patch the webhook `caBundle` again.

## Configuration

`03-config.yaml` contains the default OpenAPI settings:

* `OPENAPI_BASE_URL=http://aicp.cn-beijing-6.api.ksyun.com`. Ksyun internal accounts can use `http://aicp.cn-beijing-6.inner.api.ksyun.com`.
* `OPENAPI_AUTH_MODE=kop-sigv4`
* `OPENAPI_SERVICE=aicp`
* `OPENAPI_VERSION=2026-04-01`
* `DEFAULT_OPENAPI_CREDENTIAL_SECRET=sandbox-openapi-credentials`

Business namespaces still need their own OpenAPI AK/SK Secret. See:

```text
config/credentials/credentials.example.yaml
```

## Helm Alternative

For regular installation and upgrades, Helm is recommended:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace
```

To use a custom image, add `--set image.repository=my-registry.example.com/sandbox-operator --set image.tag=v0.1.0`.

The chart generates a self-signed webhook certificate by default. To use cert-manager:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```
