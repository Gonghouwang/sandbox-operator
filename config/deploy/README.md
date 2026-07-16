# Raw Kubernetes Deployment Resources

This directory contains operator deployment resources that do not depend on Helm:

- `00-namespace.yaml`: operator namespace.
- `01-crd.yaml`: `SandboxTemplate`, `Sandbox`, and `SandboxClaim` CRDs.
- `02-rbac.yaml`: ServiceAccount, ClusterRole, and ClusterRoleBinding.
- `03-config.yaml`: operator configuration ConfigMap.
- `04-manager.yaml`: operator Deployment.
- `05-webhook.yaml`: webhook Service, MutatingWebhookConfiguration, and ValidatingWebhookConfiguration.

## Deploy

From the repository root, run:

```bash
make deploy
```

The command applies these resources in the required order, creates the webhook TLS Secret, and patches the webhook CA bundle. It uses the public image `hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707`; no image-pull credential is needed. To use your own image, override it when deploying:

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
3. Patches the CA bundle into the MutatingWebhookConfiguration and ValidatingWebhookConfiguration.

If you replace the webhook certificate manually, you must patch the webhook `caBundle` again.

## Configuration

`03-config.yaml` contains the default OpenAPI configuration:

- `OPENAPI_BASE_URL=http://aicp.cn-beijing-6.api.ksyun.com`. Ksyun internal accounts can use `http://aicp.cn-beijing-6.inner.api.ksyun.com`.
- `OPENAPI_AUTH_MODE=kop-sigv4`
- `OPENAPI_SERVICE=aicp`
- `OPENAPI_VERSION=2026-04-01`
- `DEFAULT_OPENAPI_CREDENTIAL_SECRET=sandbox-openapi-credentials`

Business namespaces still need an OpenAPI AK/SK Secret. See the example in:

```text
config/credentials/credentials.example.yaml
```

## Helm Deployment

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace
```

To use a custom image, add `--set image.repository=my-registry.example.com/sandbox-operator --set image.tag=v0.1.0`.

The chart generates a self-signed webhook certificate by default. To use cert-manager instead:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```
