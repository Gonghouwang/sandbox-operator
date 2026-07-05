# Sandbox Operator 集群内 CR 处理流程与使用说明

本文基于当前代码实现说明 `SandboxTemplate`、`Sandbox`、`SandboxClaim` 在集群内创建、更新、删除时的完整处理流程，以及 Sandbox OpenAPI 侧变更如何同步回 Kubernetes CR。

## 1. 核心对象与组件

### 1.1 CRD 对象

当前 API Group 为：

```text
sandbox.kce.ksyun.com/v1alpha1
```

包含三个命名空间级资源：

| Kind | 简写 | 作用 |
| --- | --- | --- |
| `SandboxTemplate` | `stpl` | 声明和同步 Sandbox OpenAPI 模板。 |
| `Sandbox` | `sbx` | 声明和同步单个沙箱实例。 |
| `SandboxClaim` | `sbxc` | 批量申请沙箱实例，并聚合子 `Sandbox` 状态。 |

### 1.2 主要组件

| 组件 | 代码位置 | 职责 |
| --- | --- | --- |
| Mutating Webhook | `internal/webhook/handlers.go` | 拦截 CR 创建请求，先调用 OpenAPI 创建远端资源，再把远端 ID 写入受保护 annotation。 |
| Validating Webhook | `internal/webhook/handlers.go` | 拦截 CR 创建、更新、删除请求，做校验；更新时同步调用 OpenAPI 更新远端资源。 |
| Reconciler | `internal/controller/controllers.go` | 给 CR 添加 finalizer；按 annotation 绑定远端 ID；从 OpenAPI 拉取详情并回写 `spec/status`；删除时调用 OpenAPI 删除远端资源。 |
| Poller | `internal/controller/poller.go` | 周期性扫描 OpenAPI，把控制台或其他客户端直接创建、更新、删除的远端资源同步回 CR。 |
| Mapper | `internal/mapper/mapper.go` | 负责 CR 字段和 OpenAPI 请求/响应字段互转。 |
| Credential Manager | `internal/credentials/manager.go` | 从业务 Namespace 读取 OpenAPI、KS3、KPFS、Klog、镜像仓库等 Secret。 |

### 1.3 受保护 annotation

平台资源 ID 不写入 `status`，而是由 operator 维护在 annotation 中：

```text
sandbox.kce.ksyun.com/template-id
sandbox.kce.ksyun.com/sandbox-id
sandbox.kce.ksyun.com/sandbox-ids
sandbox.kce.ksyun.com/endpoint
sandbox.kce.ksyun.com/token
```

普通用户不能在创建时预设这些 annotation，也不能在更新时修改或删除它们。operator 自己使用的 ServiceAccount 会被 webhook 放行，避免同步回写 CR 时再次触发 OpenAPI 写操作。

## 2. 总体数据流

当前实现以 OpenAPI 作为模板和沙箱实例真实状态的权威来源。

```text
用户 kubectl/client-go
        |
        v
Kubernetes APIServer
        |
        v
Mutating/Validating Webhook  -- 创建/更新 -->  Sandbox OpenAPI
        |
        v
CR 入库
        |
        v
Reconciler  -- Get/Delete --> Sandbox OpenAPI
        |
        v
CR spec/status 回写

Poller 周期扫描 Sandbox OpenAPI
        |
        v
创建、更新或删除集群内 CR
```

写路径由 webhook 同步调用 OpenAPI，读路径和最终状态收敛由 Reconciler/Poller 从 OpenAPI 拉取详情后回写 CR。

## 3. 创建流程

### 3.1 创建 SandboxTemplate

用户创建 `SandboxTemplate` 时，流程如下：

1. 请求进入 mutating webhook。
2. webhook 检查用户没有预设受保护 annotation。
3. 校验 `spec.type`、`spec.access` 等必填字段。
4. 从 CR 所在 Namespace 读取 OpenAPI Secret。默认 Secret 名为 `sandbox-openapi-credentials`，也可以通过 `spec.openapiCredentialRef.name` 指定。
5. 如果模板引用了镜像仓库、KS3、KPFS、Klog 等运行时凭据，webhook 从同一 Namespace 读取对应 Secret。
6. mapper 将 CR 转换为 `CreateSandboxTemplate` 请求。
7. webhook 调用 OpenAPI `CreateSandboxTemplate`。
8. OpenAPI 成功返回后，webhook 把远端模板 ID 写入 `sandbox.kce.ksyun.com/template-id` annotation。
9. APIServer 将带 annotation 的 CR 持久化。
10. Reconciler 监听到 CR 后添加 `sandbox.kce.ksyun.com/sandboxtemplate-finalizer`。
11. Reconciler 根据 `template-id` 调用 `GetSandboxTemplate`，把远端详情回写到 `spec` 和 `status`。

模板状态主要由 `mapper.ApplyTemplateStatusFromOpenAPI` 写入，包括：

- `status.phase`
- `status.rawStatus`
- `status.externalUpdatedAt`
- `status.canDelete`
- `status.quota`
- `status.preheat`
- `status.createdAt`
- `status.updatedAt`
- `status.conditions`

### 3.2 创建 Sandbox

用户创建 `Sandbox` 时，流程如下：

1. 请求进入 mutating webhook。
2. webhook 检查用户没有预设受保护 annotation。
3. 校验沙箱名称在同 Namespace 内不重复。若 `spec.name` 为空，则使用 `metadata.name` 作为有效名称。
4. 解析模板引用：
   - `spec.templateRef.id` 非空时直接使用该远端模板 ID。
   - 否则读取同 Namespace 的 `SandboxTemplate`，从其 `template-id` annotation 取模板 ID。
   - `id` 和 `name` 不能同时设置。
5. 读取 OpenAPI Secret 以及 KS3/KPFS 等运行时 Secret。
6. mapper 将 CR 转换为 `StartSandboxInstance` 请求。
7. webhook 调用 OpenAPI `StartSandboxInstance`。
8. OpenAPI 成功返回后，webhook 写入：
   - `sandbox.kce.ksyun.com/template-id`
   - `sandbox.kce.ksyun.com/sandbox-id`
   - `sandbox.kce.ksyun.com/endpoint`
   - `sandbox.kce.ksyun.com/token`
9. APIServer 将带 annotation 的 CR 持久化。
10. Reconciler 添加 `sandbox.kce.ksyun.com/sandbox-finalizer`。
11. Reconciler 根据 `sandbox-id` 调用 `GetSandboxInstance`，把远端详情回写到 `spec` 和 `status`。

沙箱状态主要由 `mapper.ApplySandboxStatusFromOpenAPI` 写入，包括：

- `status.phase`
- `status.rawStatus`
- `status.timeoutSeconds`
- `status.createTime`
- `status.endTime`
- `status.endpoint`
- `status.urls`
- `status.sdnsUrls`
- `status.imageUrl`
- `status.port`
- `status.command`
- `status.env`
- `status.volumes`
- `status.observability`
- `status.template`
- `status.conditions`

其中 `status.env`、`status.volumes` 来自 `GetSandboxInstance` 响应中的 `Envs`、`Ks3MountConfig`、`KpfsMountConfig`；`status.imageUrl`、`status.port`、`status.command` 来自 OpenAPI 的 `CustomConfiguration`，在 CR 中拆成独立字段展示。`status.customConfiguration` 仅作为升级兼容字段保留，新的同步结果会清空该字段。`status.urls` 优先来自 OpenAPI `Urls`，缺失时兼容 `AccessUrl`；`status.sdnsUrls` 来自 `SdnsUrls`。当前 OpenAPI 的实例详情响应可能尚未返回 Klog 和 SDNS 访问地址；operator 已兼容 `KlogConfig`、`Urls`、`AccessUrl`、`SdnsUrls` 字段，若后续 OpenAPI 返回，会同步到 `status.observability`、`status.urls` 和 `status.sdnsUrls`。

注意：当前代码定义了 `status.token` 字段，创建时也会把 OpenAPI 返回的 token 写入 annotation，但当前同步逻辑没有把 token 回填到 `status.token`。

### 3.3 创建 SandboxClaim

`SandboxClaim` 用于一次申请多个沙箱实例。创建流程如下：

1. 请求进入 mutating webhook。
2. webhook 检查用户没有预设受保护 annotation。
3. 校验 `spec.replicas > 0`。
4. 解析模板 ID，逻辑与 `Sandbox` 类似。
5. 读取 OpenAPI Secret。
6. 按 `spec.replicas` 循环调用 `StartSandboxInstance`。
7. 每个实例启动成功后记录返回的 sandbox ID。
8. webhook 把模板 ID 和所有 sandbox ID 写入：
   - `sandbox.kce.ksyun.com/template-id`
   - `sandbox.kce.ksyun.com/sandbox-ids`
9. APIServer 持久化 Claim。
10. `SandboxClaimReconciler` 添加 `sandbox.kce.ksyun.com/sandboxclaim-finalizer`。
11. Reconciler 读取 `sandbox-ids`，按 `${claimName}-${index}` 创建子 `Sandbox` CR。
12. 每个子 `Sandbox` 继承 Claim 的模板引用、超时时间、环境变量和挂载配置，并带有对应 `sandbox-id` annotation。
13. 子 `Sandbox` 的 Reconciler/Poller 后续负责从 OpenAPI 同步实例状态。
14. `SandboxClaimReconciler` watch 子 `Sandbox` 变化并聚合 Claim 状态。

Claim 状态聚合规则：

| 条件 | Claim phase |
| --- | --- |
| 任一子 Sandbox 为 `Failed` 或 `Unhealthy` | `Failed` |
| `desired > 0` 且所有子 Sandbox 都为 `Running` | `Successful` |
| 其他情况 | `Pending` |

## 4. 更新流程

### 4.1 更新 SandboxTemplate

用户更新 `SandboxTemplate` 时，流程如下：

1. 请求进入 validating webhook。
2. webhook 校验受保护 annotation 没有被修改。
3. 校验模板基础字段。
4. 若 `spec.openapiCredentialRef.name` 发生变化，拒绝更新。当前实现认为 OpenAPI 凭据引用不可变。
5. 若新旧 `spec` 完全一致，直接放行。
6. 若 `template-id` annotation 为空，拒绝更新，要求先等待创建绑定完成。
7. 读取 OpenAPI Secret 和运行时 Secret。
8. mapper 根据新旧对象 diff 构造 `UpdateSandboxTemplate` 请求，只发送变化字段。
9. 如果更新涉及磁盘/数据盘配置且 OpenAPI 需要完整 KEC 配置，webhook 会先调用 `GetSandboxTemplate` 补齐远端已有字段。
10. 如果更新涉及 KS3/KPFS 挂载，剩余挂载项必须具备可读取的 credentialRef；删除全部挂载时会显式向 OpenAPI 发送禁用配置。
11. webhook 调用 OpenAPI `UpdateSandboxTemplate`。
12. OpenAPI 成功后 webhook 放行 Kubernetes 更新。
13. Reconciler 或 Poller 后续调用 `GetSandboxTemplate`，把 OpenAPI 最终状态回写到 CR。

因此，用户刚提交的 `spec` 可能短时间内与 OpenAPI 规范化后的返回值不同，最终以 OpenAPI 详情为准。

### 4.2 更新 Sandbox

用户更新 `Sandbox` 时，流程如下：

1. 请求进入 validating webhook。
2. webhook 校验受保护 annotation 没有被修改。
3. 校验 `spec.name` 在同 Namespace 内不重复。
4. webhook 只允许更新 `spec.name` 和 `spec.timeoutSeconds`。`spec.name` 是 Kubernetes 侧本地展示名，不触发 OpenAPI 调用。
5. 如果 `spec.timeoutSeconds` 变化，webhook 校验 `sandbox-id` annotation 非空。
6. webhook 读取 OpenAPI Secret。
7. mapper 构造 `UpdateSandboxInstance` 请求。
8. webhook 调用 OpenAPI `UpdateSandboxInstance`。
9. OpenAPI 成功后放行 Kubernetes 更新。
10. Reconciler 或 Poller 后续调用 `GetSandboxInstance`，回写 `spec.timeoutSeconds` 和 `status.timeoutSeconds` 等字段。

除 `spec.name` 和 `spec.timeoutSeconds` 外，其他 `Sandbox.spec` 字段更新会被 validating webhook 拒绝，避免用户误以为 `spec.env`、挂载或模板引用变更会同步到 OpenAPI。

### 4.3 更新 SandboxClaim

当前 `SandboxClaim` 更新流程较轻：

1. 请求进入 validating webhook。
2. webhook 校验受保护 annotation 没有被修改。
3. 更新请求被放行。
4. `SandboxClaimReconciler` 会继续根据已有 `sandbox-ids` annotation 确保子 `Sandbox` 存在，并重新聚合状态。

需要注意：当前代码没有实现 `replicas` 增大或缩小时向 OpenAPI 追加创建/删除实例的逻辑。Claim 创建时的 `sandbox-ids` 是后续子 Sandbox 数量的主要来源。

## 5. 删除流程

### 5.1 删除 SandboxTemplate

用户删除 `SandboxTemplate` 时：

1. validating webhook 放行删除，实际远端删除交给 finalizer。
2. Reconciler 看到 `deletionTimestamp` 且存在 `sandboxtemplate-finalizer`。
3. 如果 `template-id` annotation 非空，读取 OpenAPI Secret。
4. 调用 OpenAPI `DeleteSandboxTemplate`。
5. 如果 OpenAPI 返回 NotFound，视为删除已完成。
6. 删除成功后移除 finalizer。
7. APIServer 最终删除该 CR。

如果 OpenAPI 因模板仍有实例等原因拒绝删除，finalizer 不会移除，CR 会停留在 Terminating，直到远端删除成功。

### 5.2 删除 Sandbox

用户删除 `Sandbox` 时：

1. validating webhook 放行删除。
2. Reconciler 看到 `deletionTimestamp` 且存在 `sandbox-finalizer`。
3. 如果 `sandbox-id` annotation 非空，读取 OpenAPI Secret。
4. 调用 OpenAPI `DeleteSandboxInstance`，请求体中包含该实例 ID。
5. 如果 OpenAPI 返回 NotFound，视为删除已完成。
6. 删除成功后移除 finalizer。
7. APIServer 最终删除该 CR。

### 5.3 删除 SandboxClaim

用户删除 `SandboxClaim` 时：

1. validating webhook 放行删除。
2. `SandboxClaimReconciler` 看到 `deletionTimestamp` 且存在 `sandboxclaim-finalizer`。
3. 列出同 Namespace 内所有 `Sandbox`。
4. 删除 `spec.claimRef.name == claim.Name` 的子 `Sandbox` CR。
5. 子 `Sandbox` 各自通过自己的 finalizer 删除 OpenAPI 远端实例。
6. 子资源删除请求发出后，Claim Reconciler 移除 `sandboxclaim-finalizer`。
7. APIServer 最终删除 Claim。

## 6. OpenAPI 变更如何同步到集群内 CR

OpenAPI 到 CR 的同步有两条路径：事件驱动的 Reconciler 和周期性 Poller。

### 6.1 Reconciler 同步

Reconciler 在 CR 创建、更新或被其他组件回写后触发。对 `SandboxTemplate` 和 `Sandbox`，同步逻辑如下：

1. 从 annotation 读取远端 ID：
   - `SandboxTemplate` 读取 `template-id`
   - `Sandbox` 读取 `sandbox-id`
2. 如果远端 ID 为空，直接返回，并在创建初期短暂 requeue。
3. 读取同 Namespace 的 OpenAPI Secret。
4. 调用对应 Get 接口：
   - `GetSandboxTemplate`
   - `GetSandboxInstance`
5. 如果 OpenAPI 返回 NotFound，删除本地 CR。
6. 如果查询成功，mapper 将远端详情写回 CR `spec`。
7. 如果 `spec` 有变化，调用 Kubernetes `Update`。
8. mapper 将远端详情写回 CR `status`。
9. 设置 `Synced=True` condition。
10. 如果 `status` 有变化，调用 `Status().Update`。

Reconciler 主要提供事件触发后的快速收敛。

### 6.2 Poller 周期同步

Poller 在 manager 中作为 runnable 启动，默认每 `30s` 执行一轮。配置项来自 `sandbox-operator-config` 或启动参数：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `POLL_INTERVAL` | `30s` | 轮询间隔。 |
| `POLL_PAGE_SIZE` | `100` | OpenAPI 分页大小，最大截断为 100。 |
| `MAX_CONCURRENT_NAMESPACES` | `5` | 并发同步 Namespace 数量。 |
| `SYNC_NAMESPACES` | 空 | 为空时扫描所有 Namespace；非空时只扫描逗号分隔的指定 Namespace。 |

每轮同步流程：

1. 确定待同步 Namespace。
2. 对每个 Namespace 读取默认 OpenAPI Secret。没有 Secret 时跳过该 Namespace。
3. 同步模板。
4. 同步沙箱实例。

### 6.3 模板同步

对每个 Namespace，Poller 同步 `SandboxTemplate` 的流程如下：

1. 列出本地 `SandboxTemplate`。
2. 对每个带 `template-id` annotation 的本地 CR 调用 `GetSandboxTemplate`。
3. 查询成功后，回写 CR `spec/status`。
4. 查询返回 NotFound 时，删除本地 CR。
5. 当前 manager 启动时将 `AdoptExternal` 固定为 `true`，因此 Poller 会调用 `GetSandboxTemplateList` 列出远端模板。
6. 对远端存在但本地未绑定的模板，再调用 `GetSandboxTemplate` 获取详情。
7. 创建新的 `SandboxTemplate` CR，并写入：
   - `sandbox.kce.ksyun.com/adopted: "true"` label
   - `sandbox.kce.ksyun.com/template-id` annotation
8. 回写 `spec/status`。

远端模板 adoption 命名规则：

- 优先使用 OpenAPI 模板名作为 CR 名。
- 如果 OpenAPI 模板名不是合法 Kubernetes DNS1123 subdomain，则对模板名做小写、替换非法字符等清洗后作为 CR 名。
- 只有模板名为空时，才使用模板 ID 经过清洗后的名称。
- 如果同 Namespace 已有同名 CR，则追加后缀生成唯一名称。

### 6.4 沙箱实例同步

对每个 Namespace，Poller 同步 `Sandbox` 的流程如下：

1. 列出本地 `Sandbox`。
2. 对每个带 `sandbox-id` annotation 的本地 CR 调用 `GetSandboxInstance`。
3. 查询成功后，回写 CR `spec/status`。
4. 查询返回 NotFound 时，删除本地 CR。
5. 因 `AdoptExternal=true`，Poller 会按多个状态调用 `GetSandboxInstanceList` 并去重。
6. 当前状态列表为：

```text
空状态, STARTING, RUNNING, KILLING, FAILED, UNHEALTHY, PAUSED, RESUMING
```

7. 对远端存在但本地未绑定的沙箱实例创建新的 `Sandbox` CR，并写入：
   - `sandbox.kce.ksyun.com/adopted: "true"` label
   - `sandbox.kce.ksyun.com/sandbox-id` annotation
   - `sandbox.kce.ksyun.com/template-id` annotation
8. 回写 `spec/status`。

远端沙箱 adoption 命名规则：

- 如果 OpenAPI 返回的沙箱名合法，则使用沙箱名。
- 否则使用沙箱 ID 经过清洗后的名称。
- 如果同 Namespace 已有同名 CR，则追加后缀生成唯一名称。

沙箱的 `metadata.name` 是 Kubernetes 对象名，用于寻址 CR，创建后不应作为业务名修改；`spec.name` 是 Kubernetes 侧可修改的沙箱展示名，不传给当前 OpenAPI。OpenAPI 创建沙箱时没有名称时，operator 默认让 `spec.name` 使用沙箱 ID；用户后续修改 `Sandbox.spec.name` 后，OpenAPI 同步不会覆盖该本地名称。

### 6.5 同步一致性语义

当前实现是最终一致模型：

- 用户通过 CR 发起创建、更新时，webhook 会先同步调用 OpenAPI。
- Reconciler 和 Poller 会以 OpenAPI 返回的最终详情回写 CR。
- OpenAPI 控制台或其他客户端直接修改资源后，Poller 会在下一轮同步回 CR。
- 如果本地 CR 绑定的远端资源在 OpenAPI 侧被删除，Reconciler/Poller 会删除本地 CR。
- Kubernetes update conflict 会被忽略，依赖后续事件或下一轮 Poller 再次收敛。
- 凭据 Secret 不会从 OpenAPI 反写到 Kubernetes。OpenAPI 返回的脱敏凭据摘要只可能进入 `status.credentialDrift` 这类状态字段。

## 7. 凭据要求

### 7.1 OpenAPI Secret

业务 CR 所在 Namespace 必须存在 OpenAPI Secret。默认名称：

```text
sandbox-openapi-credentials
```

字段要求：

| key | 必填 | 说明 |
| --- | --- | --- |
| `accessKeyId` | 是 | OpenAPI AK。 |
| `secretAccessKey` | 是 | OpenAPI SK。 |
| `accountId` | 视认证模式而定 | KOP SigV4 模式下会写入请求参数和 Header。 |
| `region` | 是 | OpenAPI 区域。 |

示例：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sandbox-openapi-credentials
  namespace: sandbox-demo
type: Opaque
stringData:
  accessKeyId: "AKxxxxxxxx"
  secretAccessKey: "SKxxxxxxxx"
  accountId: "2000123456"
  region: "cn-beijing-6"
```

### 7.2 运行时 Secret

KS3、KPFS、Klog 等运行时 Secret 必须与引用它们的 CR 在同一 Namespace。

Opaque Secret 通用字段：

```yaml
stringData:
  accessKey: "AKxxxxxxxx"
  secretAccessKey: "SKxxxxxxxx"
  token: ""
```

镜像仓库 Secret 支持 `kubernetes.io/dockerconfigjson`，字段为 `.dockerconfigjson`。

## 8. 部署说明

### 8.1 使用普通 Kubernetes manifests 部署

前置要求：

- 可访问目标 Kubernetes 集群的 `kubectl`
- 可用的 operator 镜像
- 本地有 `openssl`，用于生成 webhook 自签证书

构建镜像：

```bash
docker build -t sandbox-operator:latest .
```

部署：

```bash
IMAGE=sandbox-operator:latest ./scripts/deploy.sh
```

脚本会执行以下操作：

1. 应用 `config/deploy/00-namespace.yaml`
2. 应用 `config/deploy/01-crd.yaml`
3. 应用 `config/deploy/02-rbac.yaml`
4. 应用 `config/deploy/03-config.yaml`
5. 使用 `openssl` 生成 webhook CA 和 serving cert
6. 创建 `sandbox-operator-webhook-server-cert` TLS Secret
7. 应用 `config/deploy/04-manager.yaml`
8. 设置 Deployment 镜像
9. 应用 `config/deploy/05-webhook.yaml`
10. patch mutating/validating webhook 的 `caBundle`
11. 等待 `sandbox-operator` Deployment rollout 完成

验证：

```bash
kubectl get pods -n sandbox-operator-system
kubectl get crd sandboxtemplates.sandbox.kce.ksyun.com
kubectl get crd sandboxes.sandbox.kce.ksyun.com
kubectl get crd sandboxclaims.sandbox.kce.ksyun.com
kubectl get mutatingwebhookconfiguration sandbox-operator-mutating-webhook
kubectl get validatingwebhookconfiguration sandbox-operator-validating-webhook
```

卸载：

```bash
./scripts/undeploy.sh
```

### 8.2 使用 Helm 部署

使用 chart 默认配置部署：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set image.repository=hub.kce.ksyun.com/sandbox/sandbox-operator \
  --set image.tag=v20260630-0f7fca74
```

默认 chart 会使用 Helm 生成自签 webhook 证书。若要使用 cert-manager：

```bash
helm upgrade --install sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --create-namespace \
  --set certManager.enabled=true \
  --set webhook.selfSigned.enabled=false
```

常用 Helm 配置：

| values key | 默认值 | 说明 |
| --- | --- | --- |
| `namespace` | `sandbox-operator-system` | operator 所在 Namespace。 |
| `image.repository` | `hub.kce.ksyun.com/sandbox/sandbox-operator` | 镜像仓库。 |
| `image.tag` | `v20260630-0f7fca74` | 镜像 tag。 |
| `config.openapiBaseURL` | `http://aicp.cn-beijing-6.inner.api.ksyun.com` | OpenAPI endpoint。 |
| `config.openapiAuthMode` | `kop-sigv4` | OpenAPI 认证模式。 |
| `config.defaultOpenAPICredentialSecret` | `sandbox-openapi-credentials` | 默认 OpenAPI Secret 名。 |
| `config.pollInterval` | `30s` | Poller 周期。 |
| `config.syncNamespaces` | 空 | 限定同步 Namespace，空表示扫描所有 Namespace。 |

升级：

```bash
helm upgrade sandbox-operator charts/sandbox-operator \
  -n sandbox-operator-system \
  --set image.tag=<new-tag>
```

卸载：

```bash
helm uninstall sandbox-operator -n sandbox-operator-system
```

## 9. 使用说明

### 9.1 创建业务 Namespace 和 OpenAPI Secret

```bash
kubectl create namespace sandbox-demo

kubectl -n sandbox-demo create secret generic sandbox-openapi-credentials \
  --from-literal=accessKeyId='<AK>' \
  --from-literal=secretAccessKey='<SK>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

如需要 KS3/KPFS 挂载：

```bash
kubectl -n sandbox-demo create secret generic ks3-credential \
  --from-literal=accessKey='<KS3_AK>' \
  --from-literal=secretAccessKey='<KS3_SK>' \
  --from-literal=token=''

kubectl -n sandbox-demo create secret generic kpfs-credential \
  --from-literal=accessKey='<KPFS_AK>' \
  --from-literal=secretAccessKey='<KPFS_SK>' \
  --from-literal=token=''
```

### 9.2 创建 SandboxTemplate

示例：

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxTemplate
metadata:
  name: custom-app
  namespace: sandbox-demo
spec:
  description: "custom app runtime"
  type: Custom
  access: Private
  template:
    spec:
      image:
        source: Public
        image: "hub.kce.ksyun.com/sandbox/aio:v20260608"
      resources:
        cpu: "2"
        memory: 4096Mi
        disk: 20480Mi
      ports:
        - name: http
          containerPort: 8080
          protocol: TCP
      startCommand: "python /home/user/app.py"
      env:
        - name: APP_ENV
          value: "prod"
      pool:
        targetSize: 1
```

应用并查看：

```bash
kubectl apply -f template.yaml
kubectl get sandboxtemplate -n sandbox-demo custom-app -o yaml
```

检查远端模板 ID：

```bash
kubectl get sandboxtemplate -n sandbox-demo custom-app \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/template-id}{"\n"}'
```

### 9.3 创建 Sandbox

使用模板名创建：

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: Sandbox
metadata:
  name: demo-sandbox
  namespace: sandbox-demo
spec:
  name: demo-sandbox
  templateRef:
    name: custom-app
  timeoutSeconds: 1800
```

应用并查看：

```bash
kubectl apply -f sandbox.yaml
kubectl get sandbox -n sandbox-demo demo-sandbox -o yaml
```

检查远端沙箱 ID：

```bash
kubectl get sandbox -n sandbox-demo demo-sandbox \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/sandbox-id}{"\n"}'
```

也可以直接使用远端模板 ID：

```yaml
spec:
  templateRef:
    id: "<template-id>"
  timeoutSeconds: 1800
```

### 9.4 创建 SandboxClaim

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxClaim
metadata:
  name: demo-claim
  namespace: sandbox-demo
spec:
  replicas: 2
  templateRef:
    name: custom-app
  timeoutSeconds: 1800
```

应用并查看：

```bash
kubectl apply -f claim.yaml
kubectl get sandboxclaim -n sandbox-demo demo-claim -o yaml
kubectl get sandbox -n sandbox-demo -l sandbox.kce.ksyun.com/claim=demo-claim
```

### 9.5 更新模板和沙箱

更新模板描述：

```bash
kubectl patch sandboxtemplate -n sandbox-demo custom-app --type=merge -p '{
  "spec": {
    "description": "updated description"
  }
}'
```

更新沙箱超时时间：

```bash
kubectl patch sandbox -n sandbox-demo demo-sandbox --type=merge -p '{
  "spec": {
    "timeoutSeconds": 3600
  }
}'
```

### 9.6 删除资源

删除单个沙箱：

```bash
kubectl delete sandbox -n sandbox-demo demo-sandbox
```

删除 Claim 以及它管理的子 Sandbox：

```bash
kubectl delete sandboxclaim -n sandbox-demo demo-claim
```

删除模板：

```bash
kubectl delete sandboxtemplate -n sandbox-demo custom-app
```

删除会通过 finalizer 级联删除 OpenAPI 远端资源。如果 CR 长时间停留在 Terminating，应先查看 operator 日志确认 OpenAPI 删除失败原因。

### 9.7 常用排查命令

查看所有资源：

```bash
kubectl get sandboxtemplates,sandboxes,sandboxclaims -n sandbox-demo -o wide
```

查看 operator 日志：

```bash
kubectl logs -n sandbox-operator-system deploy/sandbox-operator -f
```

查看配置：

```bash
kubectl get configmap -n sandbox-operator-system sandbox-operator-config -o yaml
```

查看 webhook：

```bash
kubectl get mutatingwebhookconfiguration sandbox-operator-mutating-webhook -o yaml
kubectl get validatingwebhookconfiguration sandbox-operator-validating-webhook -o yaml
```

检查模板和沙箱绑定 ID：

```bash
kubectl get sandboxtemplates -n sandbox-demo \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.sandbox\.kce\.ksyun\.com/template-id}{"\n"}{end}'

kubectl get sandboxes -n sandbox-demo \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.sandbox\.kce\.ksyun\.com/sandbox-id}{"\n"}{end}'
```

## 10. 注意事项

- OpenAPI Secret 必须存在于业务 CR 所在 Namespace。
- KS3/KPFS/Klog/镜像仓库等运行时 Secret 也必须存在于业务 CR 所在 Namespace。
- 不要手工设置、修改或删除 `sandbox.kce.ksyun.com/*` 受保护 annotation。
- 当前 `Sandbox` 更新主要支持 `spec.timeoutSeconds` 同步到 OpenAPI。
- 当前 `SandboxClaim` 不支持通过更新 `spec.replicas` 动态扩缩容远端实例。
- OpenAPI 是最终状态来源，控制台侧变更会在下一轮 Poller 中同步回 CR。
- 如果 OpenAPI 侧资源被删除，本地绑定的 CR 也会被 operator 删除。
- 凭据内容不会从 OpenAPI 反写为 Kubernetes Secret。
