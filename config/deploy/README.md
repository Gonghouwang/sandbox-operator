# 原生 Kubernetes 部署资源

本目录包含不依赖 Helm 的 operator 部署资源：

- `00-namespace.yaml`：operator 命名空间。
- `01-crd.yaml`：`SandboxTemplate`、`Sandbox`、`SandboxClaim` CRD。
- `02-rbac.yaml`：ServiceAccount、ClusterRole、ClusterRoleBinding。
- `03-config.yaml`：operator 配置 ConfigMap。
- `04-manager.yaml`：operator Deployment。
- `05-webhook.yaml`：webhook Service、MutatingWebhookConfiguration、ValidatingWebhookConfiguration。

## 应用顺序

```bash
kubectl apply -f 00-namespace.yaml
kubectl apply -f 01-crd.yaml
kubectl apply -f 02-rbac.yaml
kubectl apply -f 03-config.yaml

# webhook TLS Secret 和 caBundle 由 scripts/deploy.sh 自动生成和 patch。

kubectl apply -f 04-manager.yaml
kubectl apply -f 05-webhook.yaml
```

推荐直接使用脚本：

```bash
./scripts/build-image.sh sandbox-operator:latest
IMAGE=sandbox-operator:latest ./scripts/deploy.sh
```

卸载：

```bash
./scripts/undeploy.sh
```

## Webhook 证书

原生 manifest 部署不依赖 cert-manager。`scripts/deploy.sh` 会：

1. 使用 `openssl` 生成自签 CA 和 serving certificate。
2. 将 serving certificate 写入 `sandbox-operator-webhook-server-cert` Secret。
3. 将 CA bundle 写入 MutatingWebhookConfiguration 和 ValidatingWebhookConfiguration。

如果手工替换 webhook 证书，需要重新 patch webhook caBundle。

## 配置

`03-config.yaml` 中包含默认 OpenAPI 配置：

- `OPENAPI_AUTH_MODE=kop-sigv4`
- `OPENAPI_SERVICE=aicp`
- `OPENAPI_VERSION=2026-04-01`
- `DEFAULT_OPENAPI_CREDENTIAL_SECRET=sandbox-openapi-credentials`

业务命名空间仍需创建 OpenAPI AK/SK Secret。示例见：

```text
config/credentials/credentials.example.yaml
```

## Helm 部署

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=sandbox-operator \
  --set image.tag=latest
```

Chart 默认使用 Helm 模板生成自签 webhook 证书。若要使用 cert-manager：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```
