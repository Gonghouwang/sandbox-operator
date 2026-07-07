# Raw Kubernetes Deployment Resources

This directory contains operator deployment resources that do not depend on Helm:

- `00-namespace.yaml`: operator namespace.
- `01-crd.yaml`: `SandboxTemplate`, `Sandbox`, and `SandboxClaim` CRDs.
- `02-rbac.yaml`: ServiceAccount, ClusterRole, and ClusterRoleBinding.
- `03-config.yaml`: operator configuration ConfigMap.
- `04-manager.yaml`: operator Deployment.
- `05-webhook.yaml`: webhook Service, MutatingWebhookConfiguration, and ValidatingWebhookConfiguration.

## Apply Order

```bash
kubectl apply -f 00-namespace.yaml
kubectl apply -f 01-crd.yaml
kubectl apply -f 02-rbac.yaml
kubectl apply -f 03-config.yaml

# The webhook TLS Secret and caBundle are generated/patched automatically by scripts/deploy.sh.

kubectl apply -f 04-manager.yaml
kubectl apply -f 05-webhook.yaml
```

It is recommended to use the provided targets instead:

```bash
make docker-build IMG=sandbox-operator:latest
make deploy IMG=sandbox-operator:latest
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
  --create-namespace \
  --set image.repository=sandbox-operator \
  --set image.tag=latest
```

The chart generates a self-signed webhook certificate by default. To use cert-manager instead:

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```
