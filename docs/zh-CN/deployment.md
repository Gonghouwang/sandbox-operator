# 部署说明

本文说明如何部署 Sandbox Operator。

## 1. 部署内容

operator 部署后包含以下 Kubernetes 资源：

- `Namespace`：默认 `sandbox-operator-system`。
- CRD：`SandboxTemplate`、`Sandbox`、`SandboxClaim`。
- RBAC：operator 使用的 ServiceAccount、ClusterRole、ClusterRoleBinding。
- ConfigMap：operator 配置。
- Deployment：operator manager。
- Service：webhook 服务。
- MutatingWebhookConfiguration：创建 CR 时调用 OpenAPI 并注入平台 ID。
- ValidatingWebhookConfiguration：校验 CR 更新、删除等操作。
- Webhook TLS Secret：webhook 服务证书。

## 2. 使用 Helm 部署

推荐使用 Helm：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=sandbox-operator \
  --set image.tag=latest
```

如果镜像在私有仓库，请配置镜像地址和 pull secret：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository='<IMAGE_REPOSITORY>' \
  --set image.tag='<IMAGE_TAG>' \
  --set imagePullSecrets[0].name='<IMAGE_PULL_SECRET>'
```

默认情况下，Chart 会使用 Helm 模板生成自签 webhook 证书。

如果集群已安装 cert-manager，可以改用 cert-manager 生成证书：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```

## 3. 使用原生 Manifest 部署

构建镜像：

```bash
make docker-build IMG=sandbox-operator:latest
```

部署：

```bash
make deploy IMG=sandbox-operator:latest
```

脚本会：

1. 应用 `config/deploy` 下的 CRD、RBAC、ConfigMap、Deployment 和 webhook 配置。
2. 使用 `openssl` 生成 webhook 自签 CA 和服务端证书。
3. 创建 `sandbox-operator-webhook-server-cert` Secret。
4. 将 CA bundle 写入 webhook configuration。
5. 等待 operator Deployment 就绪。

卸载：

```bash
make undeploy
```

## 4. Operator 配置

配置来自 `sandbox-operator-config` ConfigMap，Helm 部署时可通过 `values.yaml` 覆盖。

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `OPENAPI_BASE_URL` | `http://aicp.cn-beijing-6.inner.api.ksyun.com` | Sandbox OpenAPI 地址。 |
| `OPENAPI_AUTH_MODE` | `kop-sigv4` | OpenAPI 签名模式。 |
| `OPENAPI_SERVICE` | `aicp` | KOP Service。 |
| `OPENAPI_VERSION` | `2026-04-01` | OpenAPI 版本。 |
| `DEFAULT_OPENAPI_CREDENTIAL_SECRET` | `sandbox-openapi-credentials` | 业务命名空间中的默认 OpenAPI 凭据 Secret 名称。 |
| `POLL_INTERVAL` | `30s` | OpenAPI 轮询周期。 |
| `POLL_PAGE_SIZE` | `100` | OpenAPI 列表分页大小。 |
| `MAX_CONCURRENT_NAMESPACES` | `5` | 并发同步命名空间数量。 |
| `SYNC_NAMESPACES` | 空 | 命名空间白名单；为空时扫描存在默认 OpenAPI Secret 的命名空间。 |
| `LEADER_ELECT` | `true` | 是否启用 leader election。 |

## 5. 创建业务命名空间凭据

operator 只会同步有 OpenAPI 凭据的业务命名空间。

```bash
kubectl create namespace sandbox-demo

kubectl -n sandbox-demo create secret generic sandbox-openapi-credentials \
  --from-literal=accessKeyId='<OPENAPI_ACCESS_KEY_ID>' \
  --from-literal=secretAccessKey='<OPENAPI_SECRET_ACCESS_KEY>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

运行时凭据，例如 KS3、KPFS、Klog、镜像仓库凭据，也需要放在业务 CR 所在命名空间。

完整 Secret 示例见 [CR 示例](cr-examples.md)。

## 6. 检查部署状态

```bash
kubectl get pods -n sandbox-operator-system
kubectl get mutatingwebhookconfiguration sandbox-operator-mutating-webhook
kubectl get validatingwebhookconfiguration sandbox-operator-validating-webhook
kubectl get crd sandboxtemplates.sandbox.kce.ksyun.com
kubectl get crd sandboxes.sandbox.kce.ksyun.com
kubectl get crd sandboxclaims.sandbox.kce.ksyun.com
```

查看日志：

```bash
kubectl logs -n sandbox-operator-system deploy/sandbox-operator
```
