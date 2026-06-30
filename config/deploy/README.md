# Sandbox Operator deploy resources

Apply order:

```bash
kubectl apply -f 00-namespace.yaml
kubectl apply -f 01-crd.yaml
kubectl apply -f 02-rbac.yaml
kubectl apply -f 03-config.yaml
kubectl apply -f 04-manager.yaml
kubectl apply -f 05-webhook.yaml
```

Notes:

- `05-webhook.yaml` contains a placeholder `caBundle`. Replace it with the CA for the webhook serving certificate, or manage it through cert-manager.
- `03-config.yaml` contains KOP OpenAPI defaults: `Service=aicp`, `Version=2026-04-01`, `OPENAPI_AUTH_MODE=kop-sigv4`.
- Business namespaces still need their own `sandbox-openapi-credentials` Secret.
