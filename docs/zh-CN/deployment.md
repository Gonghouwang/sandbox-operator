# 部署 Sandbox Operator

默认使用公共镜像 `hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707`，无需镜像仓库凭据。

## 快速部署

在仓库根目录选择以下任一种方式执行。

### Helm 部署

推荐用于安装和后续升级：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace
```

### 原生 Manifest 部署

适用于不使用 Helm 的环境：

```bash
make deploy
```

该命令会创建 CRD、RBAC、ConfigMap、Deployment 和 webhook 资源，自动生成 webhook TLS 证书并等待 Deployment 就绪。

## 指定 Operator 命名空间

默认命名空间为 `sandbox-operator-system`。Helm 使用 release namespace，无需额外设置 Chart 值：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-custom \
  --create-namespace
```

原生 Manifest 部署和卸载时传入相同的 `NAMESPACE`：

```bash
make deploy NAMESPACE=sandbox-operator-custom
make undeploy NAMESPACE=sandbox-operator-custom
```

CRD、ClusterRole、ClusterRoleBinding 和 WebhookConfiguration 都是集群级资源，名称固定；同一集群只应部署一个 Sandbox Operator 实例。

## 部署条件

- 集群可以通过公网访问公共镜像仓库和 Sandbox OpenAPI。
- Helm 部署需要 `helm` 与 `kubectl`。
- 原生 Manifest 部署需要 `make`、`bash`、`kubectl` 和 `openssl`。
- 部署前应确认 `kubectl get namespaces` 可正常执行；安装需要创建 CRD、ClusterRole、ClusterRoleBinding 和 webhook 等集群级资源的权限。

## 在集群外部署

可以在任意已安装 `kubectl` 的机器上部署，不需要登录目标集群节点。将目标集群的 kubeconfig 放入本仓库后，在仓库根目录设置 `KUBECONFIG`：

```bash
export KUBECONFIG="$PWD/config/kubeconfig.yaml"
kubectl get namespaces
make deploy
```

将 `config/kubeconfig.yaml` 替换为实际文件名。`KUBECONFIG` 会被当前终端中的 `kubectl`、`helm` 和 `make deploy` 自动读取；`make deploy` 启动的部署脚本也会继承该变量。

也可以只对一次部署生效：

```bash
KUBECONFIG="$PWD/config/kubeconfig.yaml" make deploy
```

## 使用内网 OpenAPI

金山云内部账号使用内网 OpenAPI 时，Helm 部署追加：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set config.openapiBaseURL=http://aicp.cn-beijing-6.inner.api.ksyun.com
```

原生 Manifest 部署时直接覆盖部署变量，无需修改文件：

```bash
make deploy OPENAPI_BASE_URL=http://aicp.cn-beijing-6.inner.api.ksyun.com
```

重复执行该命令会滚动重启 operator，使新的 ConfigMap 地址立即生效。

## Webhook 证书

Helm 默认生成自签 webhook 证书。若集群已安装 cert-manager，可改由 cert-manager 管理：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```

## 验证部署

```bash
kubectl rollout status deployment/sandbox-operator -n sandbox-operator-system
kubectl get pods -n sandbox-operator-system
kubectl get crd sandboxtemplates.sandbox.kce.ksyun.com
kubectl logs -n sandbox-operator-system deploy/sandbox-operator
```

## 创建业务命名空间凭据

operator 只管理包含 OpenAPI 凭据 Secret 的业务命名空间。部署完成后，在需要管理的命名空间创建凭据：

```bash
kubectl create namespace sandbox-demo

kubectl -n sandbox-demo create secret generic sandbox-openapi-credentials \
  --from-literal=accessKeyId='<OPENAPI_ACCESS_KEY_ID>' \
  --from-literal=secretAccessKey='<OPENAPI_SECRET_ACCESS_KEY>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

随后 operator 会自动将账号下的模板和沙箱实例同步到该命名空间的 `SandboxTemplate` 和 `Sandbox` CR。也可主动创建 `SandboxTemplate`、`Sandbox` 或 `SandboxClaim`；前两者管理对应平台资源，`SandboxClaim` 用于一次性批量创建沙箱实例。完整 CR 和凭据示例见 [CR 示例](cr-examples.md)。

## 使用自建镜像

默认使用的是公共镜像 `hub.kce.ksyun.com/ksyun-public/sandbox-operator:v20260707`，无需镜像仓库凭据。
需要自行构建镜像时，先构建并推送到集群可访问的仓库：

```bash
make docker-build IMG=my-registry.example.com/sandbox-operator:v0.1.0
make docker-push IMG=my-registry.example.com/sandbox-operator:v0.1.0
```

原生 Manifest 部署时覆盖为指定镜像：

```bash
make deploy IMG=my-registry.example.com/sandbox-operator:v0.1.0
```

Helm 部署时覆盖镜像仓库和标签：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=my-registry.example.com/sandbox-operator \
  --set image.tag=v0.1.0
```

私有镜像仓库需要先在 operator 命名空间创建 image pull Secret：

```bash
OPERATOR_NAMESPACE=sandbox-operator-system
kubectl create namespace "${OPERATOR_NAMESPACE}"
kubectl -n "${OPERATOR_NAMESPACE}" create secret docker-registry sandbox-operator-image-pull \
  --docker-server='<REGISTRY_SERVER>' \
  --docker-username='<REGISTRY_USERNAME>' \
  --docker-password='<REGISTRY_PASSWORD>'
```

原生 Manifest 部署时传入该 Secret：

```bash
make deploy NAMESPACE="${OPERATOR_NAMESPACE}" IMG=my-registry.example.com/sandbox-operator:v0.1.0 IMAGE_PULL_SECRET=sandbox-operator-image-pull
```

Helm 部署时追加 `--set imagePullSecrets[0].name=sandbox-operator-image-pull`。

## 升级与卸载

Helm 升级复用安装命令；原生 Manifest 升级重新执行 `make deploy`。

```bash
# Helm
helm uninstall sandbox-operator -n sandbox-operator-system

# 原生 Manifest
make undeploy
```

原生 Manifest 文件和 webhook 证书处理细节见 [原生 Kubernetes 部署资源](deploy-manifests.md)。
