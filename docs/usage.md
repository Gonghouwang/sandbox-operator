# 使用说明

本文按两条链路说明 Sandbox Operator 的使用方式：

- OpenAPI 到集群 CR 同步：用户在控制台或 OpenAPI 创建、修改、删除资源，operator 将资源同步到 Kubernetes。
- 集群 CR 到 OpenAPI 同步：用户创建、修改、删除 Kubernetes CR，operator 调用 OpenAPI 操作真实沙箱模板和实例。

## 1. 基本资源

| Kind | 简写 | 说明 |
| --- | --- | --- |
| `SandboxTemplate` | `stpl` | 沙箱模板。 |
| `Sandbox` | `sbx` | 单个沙箱实例。 |
| `SandboxClaim` | `sbxc` | 一次性批量创建沙箱实例的声明。 |

每个业务命名空间需要放置 OpenAPI 凭据 Secret。默认名称是 `sandbox-openapi-credentials`，也可以在 CR 中通过 `spec.openapiCredentialRef.name` 指定其它 Secret。

```bash
kubectl create namespace sandbox-demo

kubectl create secret generic sandbox-openapi-credentials \
  -n sandbox-demo \
  --from-literal=accessKeyId='<OPENAPI_AK>' \
  --from-literal=secretAccessKey='<OPENAPI_SK>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

operator 只会同步能读取到 OpenAPI 凭据的命名空间。模板、沙箱、凭据 Secret 必须位于同一个业务命名空间。

## 2. OpenAPI 到集群 CR 同步

### 2.1 同步入口

operator 内部的 poller 会定期扫描带有 OpenAPI 凭据的命名空间，并调用 OpenAPI 查询模板和实例列表。同步结果会写入对应命名空间下的 CR：

```bash
kubectl get stpl -n sandbox-demo
kubectl get sbx -n sandbox-demo
```

OpenAPI 是权威源。控制台或 OpenAPI 侧修改资源后，下一轮同步会覆盖集群内 CR 的 `spec` 和 `status`。

### 2.2 模板同步

控制台或 OpenAPI 创建模板后，operator 会创建 `SandboxTemplate` CR。名称规则如下：

- 如果 OpenAPI 模板名是合法 Kubernetes 名称，则 CR 使用模板名。
- 如果模板名不合法，CR 使用完整模板 ID。
- 平台模板 ID 写入注解 `sandbox.kce.ksyun.com/template-id`，不写入 `status`。

查看模板：

```bash
kubectl get stpl -n sandbox-demo
kubectl get stpl -n sandbox-demo <template-name> -o yaml
```

有如下字段映射：

| OpenAPI/控制台配置 | CR 字段 |
| --- | --- |
| 模板 ID | `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` |
| 模板描述 | `spec.description` |
| 模板类型 | `spec.type` |
| 访问类型 | `spec.access` |
| 镜像 | `spec.template.spec.image` |
| CPU、内存、系统盘 | `spec.template.spec.resources` |
| KEC 机型与系统盘类型 | `spec.template.spec.kec` |
| 数据盘 | `spec.template.spec.dataDisks` |
| 端口 | `spec.template.spec.ports` |
| 启动命令 | `spec.template.spec.startCommand` |
| 环境变量 | `spec.template.spec.env` |
| KS3 挂载 | `spec.template.spec.ks3MountConfig` |
| KPFS 挂载 | `spec.template.spec.kpfsMountConfig` |
| 网络 | `spec.template.spec.networkConfig` |
| 技能配置 | `spec.template.spec.skillConfig` |
| 日志配置 | `spec.template.spec.observability` |
| 预热池目标大小 | `spec.template.spec.pool.targetSize` |
| OpenAPI 更新时间 | `status.externalUpdatedAt` |
| 是否可删除 | `status.canDelete` |
| 预热池状态 | `status.preheat` |


从 OpenAPI 同步回来的 CR 不会自动创建或回写 Kubernetes Secret。也就是说，镜像仓库、KS3、KPFS、Klog 等凭据不会从 OpenAPI 反向生成 Secret。如果后续要在集群内修改镜像或挂载相关字段，需要先在同命名空间创建相应 Secret，并在 CR 中补充 `registryCredentialRef` 或 `storageCredentialRef`。

### 2.3 沙箱实例同步

控制台或 OpenAPI 创建沙箱实例后，operator 会创建 `Sandbox` CR。名称规则如下：

- 如果实例有合法名称，CR 使用实例名称。
- 如果实例名称为空或不合法，CR 使用完整实例 ID。
- 平台实例 ID 写入注解 `sandbox.kce.ksyun.com/sandbox-id`。
- 平台模板 ID 写入注解 `sandbox.kce.ksyun.com/template-id`。

查看实例：

```bash
kubectl get sbx -n sandbox-demo
kubectl get sbx -n sandbox-demo <sandbox-name> -o yaml
```

有如下字段映射：

| OpenAPI/控制台配置 | CR 字段 |
| --- | --- |
| 实例 ID | `metadata.annotations["sandbox.kce.ksyun.com/sandbox-id"]` |
| 模板 ID | `metadata.annotations["sandbox.kce.ksyun.com/template-id"]`、`spec.templateRef.id` |
| 超时时间 | `spec.timeoutSeconds`、`status.timeoutSeconds` |
| 实例状态 | `status.phase` |
| 访问地址 | `status.endpoint`、`status.urls`、`status.accessUrl` |
| token | `status.token` |
| 镜像 | `status.imageUrl` |
| 启动命令 | `status.command` |
| 实例环境变量 | `status.env` |
| 实例 KS3 挂载 | `status.ks3MountConfig` |
| 实例 KPFS 挂载 | `status.kpfsMountConfig` |
| 结束时间 | `status.endTime` |

实例级环境变量和挂载配置会覆盖模板配置。控制台修改实例超时时间后，下一轮同步会更新 `spec.timeoutSeconds` 和 `status.timeoutSeconds`。

### 2.4 OpenAPI 删除或回收资源

OpenAPI 侧资源不存在时，operator 会删除对应 CR：

- 模板被删除后，对应 `SandboxTemplate` 会从集群删除。
- 实例被删除或过期回收后，对应 `Sandbox` 会从集群删除。

因此不要把通过 OpenAPI 同步出来的 CR 当成本地副本长期保留；它们会随 OpenAPI 权威状态变化。

## 3. 集群 CR 到 OpenAPI 同步

### 3.1 创建模板

准备 `SandboxTemplate`，完整示例见 [CR 示例](cr-examples.md#2-sandboxtemplate-完整示例)。

```bash
kubectl apply -f template.yaml
kubectl get stpl -n sandbox-demo
kubectl get stpl -n sandbox-demo full-template -o yaml
```

创建 CR 时，mutating webhook 会先调用 OpenAPI 创建真实模板，再把平台模板 ID 注入 CR 注解：

```bash
kubectl get stpl -n sandbox-demo full-template \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/template-id}{"\n"}'
```

创建带私有镜像、KS3 或 KPFS 挂载的模板前，需要先创建运行时凭据 Secret。示例见 [凭据 Secret 示例](cr-examples.md#1-凭据-secret-示例)。

### 3.2 更新模板

修改 `SandboxTemplate.spec` 时，validating webhook 会计算新旧 CR 的差异，并调用 OpenAPI 的模板更新接口。当前支持按差异更新这些字段：

| CR 字段 | 说明 |
| --- | --- |
| `spec.description` | 模板描述。 |
| `spec.access` | Private / Public。Public 模板不能配置 pool。 |
| `spec.type` | AIO / Browser / Code / Custom，大小写输入会被兼容。 |
| `spec.template.spec.image` | 镜像来源、镜像地址、镜像仓库凭据。 |
| `spec.template.spec.ports` | 暴露端口。 |
| `spec.template.spec.startCommand` | 启动命令。 |
| `spec.template.spec.resources.cpu` | CPU。 |
| `spec.template.spec.resources.memory` | 内存。 |
| `spec.template.spec.resources.disk` | 系统盘大小；会通过 KEC 配置更新。 |
| `spec.template.spec.kec` | 机型和系统盘类型。 |
| `spec.template.spec.dataDisks` | 数据盘。 |
| `spec.template.spec.env` | 模板环境变量，字段为 `name/value`。 |
| `spec.template.spec.networkConfig` | 网络配置。 |
| `spec.template.spec.skillConfig` | 技能配置。 |
| `spec.template.spec.ks3MountConfig` | KS3 挂载。 |
| `spec.template.spec.kpfsMountConfig` | KPFS 挂载。 |
| `spec.template.spec.observability` | 日志配置。 |
| `spec.template.spec.pool` | Private 模板预热池目标大小。 |

示例：

```bash
kubectl edit stpl -n sandbox-demo full-template
```

如果修改 KS3/KPFS 挂载：

- 删除全部 KS3 挂载时，可以删除 `ks3MountConfig` 或设置 `enabled: false`。
- 删除全部 KPFS 挂载时，可以删除 `kpfsMountConfig` 或设置 `enabled: false`。
- 只要更新后的 KS3 或 KPFS 仍为启用状态，就必须配置 `spec.template.spec.storageCredentialRef.name`，并确保该 Secret 存在 `accessKey` 和 `secretAccessKey`。

如果修改磁盘、数据盘或 KEC 字段，需要保证 `spec.template.spec.kec.instanceType`、`spec.template.spec.kec.systemDiskType`、`spec.template.spec.resources.disk` 三个字段完整。

### 3.3 删除模板

```bash
kubectl delete stpl -n sandbox-demo full-template
```

删除模板会先检查 OpenAPI 是否允许删除：

- 如果 `status.canDelete=false`，或者 OpenAPI 返回 `CanDelete=false`，删除请求会被 webhook 拒绝，并显示错误。
- 如果允许删除，CR 会进入删除流程，reconciler 通过 finalizer 调用 OpenAPI 删除真实模板，然后移除 finalizer。

模板下仍有关联实例时，一般需要先删除实例，再删除模板。

### 3.4 创建沙箱实例

准备 `Sandbox`，完整示例见 [CR 示例](cr-examples.md#4-sandbox-完整示例)。

```bash
kubectl apply -f sandbox.yaml
kubectl get sbx -n sandbox-demo
kubectl get sbx -n sandbox-demo full-sandbox -o yaml
```

创建 CR 时，mutating webhook 会调用 OpenAPI 启动实例，并注入这些注解：

- `sandbox.kce.ksyun.com/sandbox-id`
- `sandbox.kce.ksyun.com/template-id`
- `sandbox.kce.ksyun.com/endpoint`
- `sandbox.kce.ksyun.com/token`

随后 reconciler 会根据实例 ID 查询 OpenAPI，并回写 `status`。

`Sandbox` 支持两种创建方式：

- `spec.templateRef.name` 或 `spec.templateRef.id`：基于已有模板创建实例。
- `spec.template`：内联模板。operator 会先创建模板，再基于该模板创建实例。

### 3.5 更新沙箱实例

当前 `Sandbox` 只支持更新 `spec.timeoutSeconds`：

```bash
kubectl patch sbx -n sandbox-demo full-sandbox --type=merge -p '{
  "spec": {
    "timeoutSeconds": 3600
  }
}'
```

不支持更新 `spec.name`、`spec.templateRef`、`spec.template`、`spec.env`、`spec.ks3MountConfig`、`spec.kpfsMountConfig` 等字段。沙箱名称以 `metadata.name` 为准，`spec.name` 不应配置。

### 3.6 删除沙箱实例

```bash
kubectl delete sbx -n sandbox-demo full-sandbox
```

CR 删除后，reconciler 会通过 finalizer 调用 OpenAPI 删除真实实例，然后移除 finalizer。

### 3.7 创建 SandboxClaim

`SandboxClaim` 是一次性批量创建声明。完整示例见 [CR 示例](cr-examples.md#6-sandboxclaim-完整示例)。

```bash
kubectl apply -f claim.yaml
kubectl get sbxc -n sandbox-demo full-claim -o yaml
kubectl get sbx -n sandbox-demo -l sandbox.kce.ksyun.com/claim=full-claim
```

创建 Claim 时，webhook 会按 `spec.replicas` 调用 OpenAPI 启动多个实例。reconciler 会把这些实例物化为子 `Sandbox` CR，名称为 `<claim-name>-<index>`。

Claim 达到 `Successful` 或 `Failed` 后不再继续创建实例，也不会因为子实例被底层回收而重新补齐。`SandboxClaim` 不支持更新；如需变更副本数、超时时间、环境变量或挂载配置，需要新建一个 Claim。

删除 `SandboxClaim` 只删除声明本身，不删除已经创建出来的 `Sandbox` 和底层实例：

```bash
kubectl delete sbxc -n sandbox-demo full-claim
```

如果需要删除 Claim 创建的实例，请删除对应子 `Sandbox`。

## 4. 常用状态字段

### 4.1 SandboxTemplate

| 字段 | 说明 |
| --- | --- |
| `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` | 平台模板 ID。 |
| `status.phase` | 模板状态。 |
| `status.canDelete` | 模板是否可删除。 |
| `status.externalUpdatedAt` | OpenAPI 侧更新时间。 |
| `status.createdAt` / `status.updatedAt` | OpenAPI 侧创建和更新时间。 |
| `status.quota` | 模板配额信息。 |
| `status.preheat` | Private 模板预热池状态；Public 模板不写该字段。 |
| `status.conditions` | 同步结果和异常信息。 |

### 4.2 Sandbox

| 字段 | 说明 |
| --- | --- |
| `metadata.annotations["sandbox.kce.ksyun.com/sandbox-id"]` | 平台实例 ID。 |
| `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` | 平台模板 ID。 |
| `status.phase` | 实例状态。 |
| `status.timeoutSeconds` | 实例超时时间。 |
| `status.endpoint` | 实例入口。 |
| `status.urls` / `status.accessUrl` | 访问地址。 |
| `status.token` | 访问 token。 |
| `status.imageUrl` | 实例镜像。 |
| `status.command` | 启动命令。 |
| `status.env` | 实例环境变量。 |
| `status.ks3MountConfig` / `status.kpfsMountConfig` | 实例挂载配置。 |
| `status.endTime` | 实例结束时间。 |
| `status.conditions` | 同步结果和异常信息。 |

### 4.3 SandboxClaim

| 字段 | 说明 |
| --- | --- |
| `status.phase` | Claim 状态。 |
| `status.desired` | 期望实例数量。 |
| `status.created` | 已创建子 Sandbox 数量。 |
| `status.ready` | Running 子 Sandbox 数量。 |
| `status.failed` | Failed/Unhealthy 子 Sandbox 数量。 |
| `status.sandboxes` | 子 Sandbox 名称和状态。 |
| `status.conditions` | 处理结果和异常信息。 |

## 5. 排查命令

查看 operator 日志：

```bash
kubectl logs -n sandbox-operator-system deploy/sandbox-operator
```

查看 OpenAPI 凭据是否存在：

```bash
kubectl get secret -n sandbox-demo sandbox-openapi-credentials -o yaml
```

查看模板删除被拒绝的原因：

```bash
kubectl get stpl -n sandbox-demo full-template -o yaml
```

重点查看：

- `status.canDelete`
- `status.conditions`
- `metadata.deletionTimestamp`
- `metadata.finalizers`

查看实例与平台 ID：

```bash
kubectl get sbx -n sandbox-demo full-sandbox \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/sandbox-id}{"\n"}'
```

## 6. 常见问题

### OpenAPI 侧资源没有同步到集群

确认业务命名空间存在 OpenAPI 凭据 Secret，并且 Secret 包含 `accessKeyId`、`secretAccessKey`、`region`。operator 只扫描能读取到凭据的命名空间。

### 创建或更新挂载配置失败

KS3/KPFS 挂载需要 `storageCredentialRef` 指向同命名空间 Secret。Secret 必须包含 `accessKey` 和 `secretAccessKey`。

### 修改 Sandbox 环境变量、挂载或模板引用被拒绝

当前 `Sandbox` 只支持更新 `spec.timeoutSeconds`。其它字段只在创建实例时生效。

### Public 模板配置 pool 被拒绝

Public 模板不支持预热池。删除 `spec.template.spec.pool` 后重新提交。

### 删除模板失败

模板下仍有关联实例。先删除依赖 `Sandbox`，再删除模板。
