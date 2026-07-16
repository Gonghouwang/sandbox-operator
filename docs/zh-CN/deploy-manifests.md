# 原生 Kubernetes 部署资源

原生 manifest 位于 `config/deploy/`，包含不依赖 Helm 的 operator 部署资源：

- `00-namespace.yaml`：operator 命名空间。
- `01-crd.yaml`：`SandboxTemplate`、`Sandbox`、`SandboxClaim` CRD。
- `02-rbac.yaml`：ServiceAccount、ClusterRole、ClusterRoleBinding。
- `03-config.yaml`：operator 配置 ConfigMap。
- `04-manager.yaml`：operator Deployment。
- `05-webhook.yaml`：webhook Service、MutatingWebhookConfiguration、ValidatingWebhookConfiguration。

## 部署

在仓库根目录执行：

```bash
make deploy
```

该命令会按正确顺序应用这些资源，创建 webhook TLS Secret 并写入 webhook CA bundle。默认使用公共镜像 `hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707`，无需配置镜像拉取凭据。若使用自行构建的镜像，可在部署时覆盖：

```bash
make deploy IMG=my-registry.example.com/sandbox-operator:v0.1.0
```

若该镜像仓库为私有仓库，先在 `sandbox-operator-system` 命名空间创建 image pull Secret，再追加 `IMAGE_PULL_SECRET=<SECRET_NAME>`：

```bash
make deploy IMG=my-registry.example.com/sandbox-operator:v0.1.0 IMAGE_PULL_SECRET=sandbox-operator-image-pull
```

卸载：

```bash
make undeploy
```

## Webhook 证书

原生 manifest 部署不依赖 cert-manager。`scripts/deploy.sh` 会：

1. 使用 `openssl` 生成自签 CA 和 serving certificate。
2. 将 serving certificate 写入 `sandbox-operator-webhook-server-cert` Secret。
3. 将 CA bundle 写入 MutatingWebhookConfiguration 和 ValidatingWebhookConfiguration。

如果手工替换 webhook 证书，需要重新 patch webhook caBundle。

## 配置

`03-config.yaml` 中包含默认 OpenAPI 配置：

- `OPENAPI_BASE_URL=http://aicp.cn-beijing-6.api.ksyun.com`。金山云内部账号可改用 `http://aicp.cn-beijing-6.inner.api.ksyun.com`。
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
  --create-namespace
```

如需使用自定义镜像，追加 `--set image.repository=my-registry.example.com/sandbox-operator --set image.tag=v0.1.0`。

Chart 默认使用 Helm 模板生成自签 webhook 证书。若要使用 cert-manager：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```
