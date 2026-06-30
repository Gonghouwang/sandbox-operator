# Sandbox Operator 详细设计方案

## 1. 背景与目标

当前沙箱控制面已经通过 `sandbox-openapi` 暴露模板、实例、配额等 OpenAPI，客户希望通过 Kubernetes CRD 管理沙箱模板和沙箱实例。因此需要在当前 Go 项目中实现一套独立的 Sandbox Operator，将 Kubernetes 资源声明转换为 OpenAPI 调用，并持续把 OpenAPI 侧的真实状态同步回 CR。

本方案以根目录 `Sandbox+Operator设计文档.pdf` 为基础，并结合当前项目 `sandbox-openapi` 的真实接口、参数和响应模型进行落地设计。核心设计取舍如下：

- 写路径：用户通过 `kubectl/client-go` 创建、更新、删除 CR 时，先进入 Webhook。Webhook 调用 OpenAPI 成功后才放行请求，CR 再由 APIServer 写入 etcd。
- 读路径：Reconciler 不在普通 Watch 事件中创建或更新平台资源。OpenAPI 是模板、实例运行状态和可反推元数据的权威来源，由轮询任务拉取模板和实例信息后更新 CR `status`，并在必要时反写 CR `spec`。
- 同步边界：控制台、SDK、OpenAPI 直接创建或修改的平台资源，也要通过 Poller 同步到 Kubernetes CR。模板元数据和实例 `timeout` 等非敏感字段允许反写 `spec`；凭据类字段只报告漂移，不反写 Kubernetes Secret。
- 凭据边界：每个业务 Namespace 通过 Secret 提供 OpenAPI AK/SK、账号和地域；模板镜像、KS3、KPFS 等运行时凭据也通过 Secret 引用。

当前 `sandbox-openapi` 使用金山云 OpenAPI Action 路由，主要接口如下：

| 资源 | Action | 用途 |
| --- | --- | --- |
| 模板 | `CreateSandboxTemplate` | 创建模板 |
| 模板 | `UpdateSandboxTemplate` | 更新模板 |
| 模板 | `DeleteSandboxTemplate` | 删除模板 |
| 模板 | `GetSandboxTemplate` | 查询模板详情 |
| 模板 | `GetSandboxTemplateList` | 查询模板列表 |
| 实例 | `StartSandboxInstance` | 启动实例 |
| 实例 | `UpdateSandboxInstance` | 更新实例超时时间 |
| 实例 | `DeleteSandboxInstance` | 删除实例 |
| 实例 | `GetSandboxInstance` | 查询实例详情 |
| 实例 | `GetSandboxInstanceList` | 查询实例列表 |
| 实例 | `GetSandboxInstanceToken` | 获取实例访问 Token |
| 配额 | `GetSandboxQuota` | 查询沙箱配额 |

## 2. 总体架构

```text
kubectl/client-go
      |
      v
Kubernetes APIServer
      |
      v
Validating Webhook
      |
      | 读取 Namespace Secret
      | 调用 Sandbox OpenAPI
      v
Sandbox OpenAPI
      |
      v
平台侧模板/实例资源

Sandbox Operator Manager
  - SandboxTemplateReconciler
  - SandboxClaimReconciler
  - SandboxReconciler
  - SandboxTemplateWebhook
  - SandboxClaimWebhook
  - SandboxWebhook
  - OpenAPI Client
  - Credential Manager
  - Poller
      |
      | 周期性 List/Get OpenAPI
      v
Kubernetes CR status/spec 同步
```

### 2.1 组件职责

| 组件 | 职责 |
| --- | --- |
| `SandboxTemplateWebhook` | 拦截 `SandboxTemplate` 的 CREATE/UPDATE/DELETE，校验字段和 Secret，调用模板 OpenAPI。 |
| `SandboxClaimWebhook` | 拦截 `SandboxClaim` 的 CREATE/UPDATE/DELETE，按 `replicas` 申请实例或释放实例。 |
| `SandboxWebhook` | 拦截 `Sandbox` 的 CREATE/UPDATE/DELETE，支持手动创建实例、更新超时时间、删除实例。 |
| `SandboxTemplateReconciler` | 普通 Reconcile 只做轻量本地处理；模板状态和可反推元数据同步由 Poller 驱动。 |
| `SandboxClaimReconciler` | 汇总由 Claim 创建的 Sandbox 状态，更新 Claim `status`；不直接创建平台资源。 |
| `SandboxReconciler` | 处理 Sandbox `status` 同步、本地 CR 补齐、`timeout` 等元数据反写和 access token 更新。 |
| `Poller` | 按 Namespace 周期性调用 OpenAPI List/Get，更新或创建本地 CR，并按策略反写 `spec`。 |
| `OpenAPI Client` | 封装 Action 请求、签名、重试、错误解析和响应反序列化。 |
| `Credential Manager` | 读取 OpenAPI 凭据 Secret 和运行时凭据 Secret，做字段校验和缓存。 |
| `Mapper` | 在 Kubernetes 风格 CR 字段和 OpenAPI 字段之间转换。 |
| `Status Manager` | 统一更新 `observedGeneration`、`phase`、`conditions` 和事件。 |

### 2.2 核心原则

1. OpenAPI 是平台资源状态的权威来源。
2. 用户写 CR 触发平台写操作，平台写操作失败则拒绝 CR 写入。
3. Operator 自身写回 CR 时必须跳过 Webhook 的 OpenAPI 写操作，避免循环调用。
4. `status` 只由 Operator 写；`spec` 可以由用户写，也可以由 Operator 根据 OpenAPI 侧外部修改反写，但 Operator 写入必须跳过 Webhook 的 OpenAPI 写操作。
5. 删除默认级联到平台资源，可通过 `deletionPolicy: Retain` 改为只删 CR。
6. OpenAPI 外部修改与用户修改不做 Kubernetes 侧冲突检测，采用后写覆盖语义，底层 OpenAPI 负责并发更新一致性。
7. 所有 OpenAPI 调用必须带 `requestId`/trace 信息并记录 Kubernetes Event。

## 3. API Group 与资源

建议 API Group：

```text
sandbox.kce.ksyun.com/v1alpha1
```

命名空间级资源：

- `SandboxTemplate`：沙箱模板声明，对应 OpenAPI 模板。
- `SandboxClaim`：批量申请实例的声明，便于从模板或预热池获取一个或多个 Sandbox。
- `Sandbox`：单个沙箱实例声明，对应 OpenAPI 实例。

建议短名：

- `stpl` -> `SandboxTemplate`
- `sbxc` -> `SandboxClaim`
- `sbx` -> `Sandbox`

## 4. 凭据模型

### 4.1 OpenAPI 凭据

每个业务 Namespace 必须存在 OpenAPI 凭据 Secret：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sandbox-openapi-credentials
  namespace: sandbox-demo
  labels:
    sandbox.ksyun.com/credential-type: openapi
type: Opaque
stringData:
  accessKeyId: "AKxxxxxxxx"
  secretAccessKey: "SKxxxxxxxx"
  accountId: "2000123456"
  region: "cn-beijing-6"
```

Operator 读取顺序：

1. 如果 CR `spec.openapiCredentialRef.name` 存在，使用该 Secret。
2. 否则使用 Namespace 默认 Secret `sandbox-openapi-credentials`。

字段要求：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `accessKeyId` | 是 | OpenAPI AK |
| `secretAccessKey` | 是 | OpenAPI SK |
| `accountId` | 建议必填 | 用于多租户标识和同步过滤 |
| `region` | 是 | OpenAPI 请求地域 |

### 4.2 运行时凭据

镜像、KS3、KPFS、Klog 等凭据通过 Secret 引用，Webhook 在调用 OpenAPI 前读取并转换。

| 用途 | Secret 类型 | 字段 |
| --- | --- | --- |
| 镜像仓库 | `kubernetes.io/dockerconfigjson` | `.dockerconfigjson` |
| KS3 | `Opaque` | `accessKey`、`secretAccessKey` |
| KPFS | `Opaque` | `accessKey`、`secretAccessKey`、可选 `token` |
| Klog | `Opaque` | `accessKey`、`secretAccessKey` |

Secret 不写入 CR `status`，仅允许写入是否就绪、引用名和脱敏后的摘要。控制台或 OpenAPI 侧修改 KS3、KPFS、镜像仓库、Klog 等凭据时，Operator 不反向修改用户的 Kubernetes Secret；只在 CR `status` 中记录平台侧观察到的凭据摘要、更新时间和漂移状态。

## 5. CRD 设计

### 5.1 SandboxTemplate

`SandboxTemplate` 对应平台侧沙箱模板。`metadata.name` 映射 OpenAPI `TemplateName`，`spec` 映射创建/更新模板参数，`status` 映射 `GetSandboxTemplate` 或 `GetSandboxTemplateList` 响应。

示例：

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxTemplate
metadata:
  name: custom-app
  namespace: sandbox-demo
spec:
  description: "custom app runtime"
  category: Private
  type: Custom
  image:
    source: Public
    imageUrl: "hub.kce.ksyun.com/sandbox/aio"
    imageTag: "v20260608"
  command: "python /home/user/app.py"
  ports:
    - 8080
  resources:
    cpu: 2
    memoryGB: 4
  network:
    publicNetworkEnable: true
    privateNetworkEnable: true
    sharedInternetAccessEnable: false
    vpc:
      vpcId: vpc-xxxx
      subnetId: subnet-xxxx
  env:
    - key: APP_ENV
      value: prod
  ks3MountConfig:
    enabled: true
    credentialRef:
      name: ks3-credential
    mountPoints:
      - bucketName: bucket-a
        remotePath: /datasets
        localMountPath: /mnt/ks3/datasets
        readOnly: true
  kpfsMountConfig:
    enabled: false
  klog:
    enabled: true
  preheat:
    enabled: true
    number: 1
  instanceQuota: 10
  deletionPolicy: Delete
```

关键 Spec 字段：

| CR 字段 | 类型 | OpenAPI 字段 | 说明 |
| --- | --- | --- | --- |
| `metadata.name` | string | `TemplateName` | 模板名，需在同一 Namespace/凭据下唯一。 |
| `spec.description` | string | `Description` | 模板描述。 |
| `spec.category` | enum | `TemplateCategory` | `Public` 或 `Private`。 |
| `spec.type` | enum | `TemplateType` | `All-in-one`、`Browser`、`CodeInterpreter`、`Custom`。 |
| `spec.image.source` | enum | `ImageConfig.ImageSource` | `Public`、`Personal`、`Enterprise`。 |
| `spec.image.imageUrl` | string | `ImageConfig.ImageUrl` | 公共镜像地址。 |
| `spec.image.imageEndpoint` | string | `ImageConfig.ImageEndpoint` | 非公共镜像仓库 Endpoint。 |
| `spec.image.imageNamespace` | string | `ImageConfig.ImageNamespace` | 非公共镜像命名空间。 |
| `spec.image.imageName` | string | `ImageConfig.ImageName` | 非公共镜像名。 |
| `spec.image.imageTag` | string | `ImageConfig.ImageTag` | 镜像 Tag。 |
| `spec.image.registryInstanceId` | string | `ImageConfig.RegistryInstanceId` | 企业镜像必填。 |
| `spec.command` | string | `Command` | 启动命令。 |
| `spec.ports` | []int | `Ports` | 容器监听端口，1-65535，最多 5 个。 |
| `spec.resources.cpu` | int | `Cpu` | vCPU 核数。 |
| `spec.resources.memoryGB` | int | `Memory` | 内存 GB。 |
| `spec.kec` | object | `KecConfig` | KEC 机型和磁盘配置。 |
| `spec.env` | []object | `Envs` | 环境变量，`key/value` 映射 `Key/Value`。 |
| `spec.skill` | object | `SkillConfig` | Skills 配置。 |
| `spec.network` | object | `NetworkConfig` | 公网/私网/VPC 配置。 |
| `spec.klog.enabled` | bool | `KlogConfig.KlogEnable` | Klog 开关。 |
| `spec.kpfsMountConfig` | object | `KpfsMountConfig` | KPFS 挂载。 |
| `spec.ks3MountConfig` | object | `Ks3MountConfig` | KS3 挂载。 |
| `spec.preheat` | object | `PreheatConfig` | 预热配置。 |
| `spec.instanceQuota` | int | `InstanceQuota` | 单模板实例上限。 |
| `spec.deletionPolicy` | enum | 无 | `Delete` 或 `Retain`。 |

Status 设计：

```yaml
status:
  observedGeneration: 1
  templateID: tpl_custom_app_xxx
  phase: Ready
  rawStatus: Ready
  externalUpdatedAt: "2026-06-29T10:01:00Z"
  canDelete: true
  credentialDrift:
    ks3:
      inSync: false
      source: Console
      accessKeyIdMasked: "AK****1234"
      observedAt: "2026-06-29T10:01:00Z"
      reason: SecretNotUpdated
  klog:
    projectName: sandbox-log-project
    poolName: sandbox-log-pool
  quota:
    instanceQuota: 10
    remainingInstanceQuota: 8
    remainingSystemInstanceQuota: 20
  preheat:
    enabled: true
    number: 1
    preheatedInstanceNumber: 1
  createdAt: "2026-06-29T10:00:00Z"
  updatedAt: "2026-06-29T10:01:00Z"
  conditions:
    - type: Ready
      status: "True"
      reason: TemplateReady
      message: "Sandbox template is ready."
      lastTransitionTime: "2026-06-29T10:01:00Z"
    - type: Synced
      status: "True"
      reason: OpenAPISynced
      message: "Template status has been synced from Sandbox OpenAPI."
      lastTransitionTime: "2026-06-29T10:01:00Z"
```

Phase 映射：

| OpenAPI `Status` | CR `phase` |
| --- | --- |
| `Ready` | `Ready` |
| `Unknown` 或空 | `Unknown` |
| 创建已提交但尚未同步到 ID | `Pending` |
| OpenAPI 调用失败 | `Failed` |
| 删除中 | `Deleting` |

### 5.2 SandboxClaim

`SandboxClaim` 用于批量申请实例。它不是平台侧独立资源，而是 Kubernetes 侧的申请记录和聚合状态。Webhook 在 CREATE 时根据 `replicas` 调用 `StartSandboxInstance`，Poller/Reconciler 根据创建出的 Sandbox 更新 Claim `status`。

示例：

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxClaim
metadata:
  name: demo-claim
  namespace: sandbox-demo
spec:
  replicas: 3
  templateRef:
    name: custom-app
  timeoutSeconds: 1800
  env:
    - key: DEBUG
      value: "false"
  volumes:
    mode: Replace
    ks3MountConfig:
      enabled: true
      credentialRef:
        name: runtime-ks3-credential
      mountPoints:
        - bucketName: bucket-a
          remotePath: /datasets
          localMountPath: /mnt/ks3/datasets
          readOnly: true
  deletionPolicy: Delete
```

关键 Spec 字段：

| CR 字段 | OpenAPI 字段 | 说明 |
| --- | --- | --- |
| `spec.replicas` | 多次调用 `StartSandboxInstance` | 创建实例数量，建议最大值通过 webhook 配置限制。 |
| `spec.templateRef.id` | `TemplateId` | 直接引用平台模板 ID。 |
| `spec.templateRef.name` | 先查本地 `SandboxTemplate.status.templateID` | 引用同 Namespace 模板名。 |
| `spec.timeoutSeconds` | `Timeout` | 60-86400，默认 3600。 |
| `spec.env` | `Envs` | 实例级环境变量，覆盖模板同名变量。 |
| `spec.volumes.ks3MountConfig` | `Ks3MountConfig` | 实例级 KS3 挂载。 |
| `spec.volumes.kpfsMountConfig` | `KpfsMountConfig` | 实例级 KPFS 挂载。 |
| `spec.deletionPolicy` | 无 | 删除 Claim 时是否级联删除实例。 |

Status 设计：

```yaml
status:
  observedGeneration: 1
  phase: Successful
  replicas:
    desired: 3
    created: 3
    ready: 3
    failed: 0
    unknown: 0
  sandboxes:
    - name: demo-claim-0
      sandboxID: sbx-001
      phase: Running
      ready: true
    - name: demo-claim-1
      sandboxID: sbx-002
      phase: Running
      ready: true
  conditions:
    - type: Bound
      status: "True"
      reason: AllSandboxesCreated
      message: "All requested sandboxes have been created."
      lastTransitionTime: "2026-06-29T05:15:22Z"
    - type: Ready
      status: "True"
      reason: AllSandboxesReady
      message: "All requested sandboxes are running."
      lastTransitionTime: "2026-06-29T05:15:30Z"
```

Phase 映射：

| 条件 | `phase` |
| --- | --- |
| OpenAPI 调用中或实例尚未全部同步 | `Creating` |
| 所有实例创建成功且至少完成同步 | `Successful` |
| 任一创建调用失败且未全部满足期望 | `Failed` |
| 删除中 | `Deleting` |

### 5.3 Sandbox

`Sandbox` 对应平台侧单个实例。它有两种来源：

1. 用户手动创建 `Sandbox`。
2. 用户创建 `SandboxClaim` 后，Operator 根据 OpenAPI 返回结果补齐或创建 `Sandbox` CR。

`spec.name` 是 Kubernetes 侧的沙箱名称标识，底层 OpenAPI 当前不支持沙箱名称字段，因此创建、更新实例时都不传递该字段。它与不可变的 `metadata.name` 不同，允许用户修改，但同一 Namespace 下所有 `Sandbox.spec.name` 必须唯一，Webhook 在 CREATE/UPDATE 时校验重名。Claim 自动创建的 Sandbox 默认使用 `${claimName}-${index}` 作为 `spec.name`。

示例：

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
  env:
    - key: DEBUG
      value: "false"
  ks3MountConfig:
    enabled: true
    credentialRef:
      name: runtime-ks3-credential
    mountPoints:
      - bucketName: bucket-a
        remotePath: /datasets
        localMountPath: /mnt/ks3/datasets
        readOnly: true
  deletionPolicy: Delete
```

关键 Spec 字段：

| CR 字段 | OpenAPI 字段 | 说明 |
| --- | --- | --- |
| `spec.name` | 无 | 沙箱名，仅作为 Kubernetes 侧显示和检索标识，不传递给 OpenAPI。支持修改，但同一 Namespace 下必须唯一；未填写时默认使用 `metadata.name`。 |
| `spec.claimRef` | 无 | 记录来源 Claim。 |
| `spec.templateRef.id` | `TemplateId` | 直接引用平台模板 ID。 |
| `spec.templateRef.name` | 先解析为 `TemplateId` | 引用同 Namespace 模板名。 |
| `spec.timeoutSeconds` | `Timeout` | 创建和更新超时时间。 |
| `spec.env` | `Envs` | 实例级环境变量。 |
| `spec.ks3MountConfig` | `Ks3MountConfig` | 实例级 KS3 挂载。 |
| `spec.kpfsMountConfig` | `KpfsMountConfig` | 实例级 KPFS 挂载。 |
| `spec.deletionPolicy` | 无 | 删除 CR 时是否删除平台实例。 |

当前 OpenAPI 中 `PauseSandboxInstance` 和 `ResumeSandboxInstance` 控制器代码被注释，Operator v1alpha1 不建议暴露 `spec.paused` 作为可写能力。后续 OpenAPI 恢复暂停/恢复接口后，再增加 `spec.paused` 与对应 Webhook 逻辑。

Status 设计：

```yaml
status:
  observedGeneration: 1
  sandboxID: sbx-001
  externalUpdatedAt: "2026-06-29T05:16:00Z"
  template:
    id: tpl_custom_app_xxx
    type: Custom
    category: Private
  phase: Running
  rawStatus: RUNNING
  timeoutSeconds: 1800
  createTime: "2026-06-29T05:15:21Z"
  endTime: "2026-06-29T05:45:21Z"
  endpoint: "https://sandbox.example.com/sandbox/sbx-001"
  token: "xxxx"
  customConfiguration:
    imageUrl: "hub.kce.ksyun.com/sandbox/aio:v20260608"
    port: 8080
    command: "python /home/user/app.py"
  conditions:
    - type: Ready
      status: "True"
      reason: SandboxRunning
      message: "Sandbox is running."
      lastTransitionTime: "2026-06-29T05:15:30Z"
    - type: Synced
      status: "True"
      reason: OpenAPISynced
      message: "Sandbox status has been synced from Sandbox OpenAPI."
      lastTransitionTime: "2026-06-29T05:15:30Z"
```

Phase 映射：

| OpenAPI `Status` | CR `phase` |
| --- | --- |
| `STARTING` | `Starting` |
| `RUNNING` | `Running` |
| `KILLING` | `Deleting` |
| `FAILED` | `Failed` |
| `UNHEALTHY` | `Unhealthy` |
| `PAUSED` | `Paused` |
| `RESUMING` | `Resuming` |
| `UNKNOWN` 或空 | `Unknown` |

## 6. Webhook 设计

### 6.1 Webhook 配置

使用 Validating Webhook：

- `failurePolicy: Fail`
- `sideEffects: NoneOnDryRun`
- `timeoutSeconds: 10`
- `matchPolicy: Equivalent`
- `admissionReviewVersions: ["v1"]`

Webhook path：

| 资源 | Path |
| --- | --- |
| `SandboxTemplate` | `/validate-sandbox-kce-ksyun-com-v1alpha1-sandboxtemplate` |
| `SandboxClaim` | `/validate-sandbox-kce-ksyun-com-v1alpha1-sandboxclaim` |
| `Sandbox` | `/validate-sandbox-kce-ksyun-com-v1alpha1-sandbox` |

Operator 自身写回 CR 时跳过 OpenAPI 调用：

```go
const operatorSAUsername = "system:serviceaccount:sandbox-operator-system:sandbox-operator"

func isOperatorRequest(req admission.Request) bool {
    return req.UserInfo.Username == operatorSAUsername
}
```

建议额外支持按 Group 判断：

- `system:masters` 不默认跳过。
- 只有 Operator ServiceAccount 的 status/spec 同步请求跳过平台写操作。
- 用户若能冒用该 ServiceAccount 会破坏一致性，部署时必须通过 RBAC 禁止业务用户使用。

### 6.2 Validating Webhook 与 ID 回填限制

Validating Webhook 不能修改 admission 中的对象，因此无法把 `CreateSandboxTemplate` 返回的 `TemplateId` 或 `StartSandboxInstance` 返回的 `InstanceId` 直接写入被创建的 CR。

本设计采用以下约束：

1. Webhook 调用 OpenAPI 成功后放行。
2. Poller 通过 `GetSandboxTemplateList`、`GetSandboxInstanceList` 找到同 Namespace/凭据下的资源，再写入 CR `status.templateID` 或 `status.sandboxID`。
3. CR `metadata.name` 必须作为 OpenAPI `TemplateName` 的来源，并要求在同一 OpenAPI 账号/地域/Namespace 下唯一。
4. 对 `SandboxClaim` 创建出的实例，Webhook 应记录实例创建结果到内部临时操作记录，Poller/Reconciler 再创建或绑定 `Sandbox` CR。

`OperationRecord` 不是面向用户的业务资源，而是 Operator 内部的短期绑定缓存。原因是 Validating Webhook 只能允许或拒绝 admission，不能直接把 `CreateSandboxTemplate` / `StartSandboxInstance` 返回的 ID 写入 CR `status`。因此 webhook 需要把返回的 `TemplateId`、`InstanceId` 暂存到同 Namespace 的 ConfigMap，后续 Poller/Reconciler 再消费该记录并写入 CR `status`。该记录不存储 token、AK/SK、密码等敏感信息。

推荐用 ConfigMap 存储：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sandbox-op-demo-claim
  namespace: sandbox-demo
  labels:
    sandbox.kce.ksyun.com/operation: "true"
data:
  kind: SandboxClaim
  name: demo-claim
  generation: "1"
  action: Create
  sandboxIDs: '["sbx-001","sbx-002","sbx-003"]'
```

如果不引入操作记录，则 Claim 创建后需要完全依赖 `GetSandboxInstanceList(TemplateId)` 与时间窗口、命名规则进行匹配，可靠性较弱。

### 6.3 SandboxTemplateWebhook

#### CREATE

流程：

1. 跳过 Operator ServiceAccount 请求。
2. 校验 `metadata.name` 与 `spec`。
3. 读取 OpenAPI 凭据 Secret。
4. 读取镜像、KS3、KPFS、Klog 等引用 Secret。
5. 查询同名模板，避免 OpenAPI 侧已有同名资源导致误绑定。
6. 转换为 `CreateSandboxTemplate` 请求。
7. 调用 OpenAPI。
8. 成功则允许 CR 写入；失败则拒绝并返回 OpenAPI 错误。

主要校验：

- `category=Public` 时，`type` 必须为 `All-in-one`、`Browser` 或 `CodeInterpreter`。
- `category=Private` 时，`image` 与 `network` 必填。
- 私有模板且镜像源非 Public 时，不支持 Skills。
- 端口范围 1-65535，最多 5 个。
- CPU/Memory 使用 OpenAPI 支持的组合。
- KS3/KPFS 任一启用时，相关 AK/SK Secret 必须存在。
- 挂载路径必须以 `/` 开头，不允许与禁止路径冲突。
- `preheat.number` 不能超过 `instanceQuota`。

#### UPDATE

流程：

1. 跳过 Operator ServiceAccount 请求。
2. 禁止修改不可变字段，建议 `metadata.name`、`spec.openapiCredentialRef` 不可变。
3. 如果 `status.templateID` 为空，先按名称查询并绑定；仍为空则拒绝更新。
4. 根据新旧对象计算是否需要调用 `UpdateSandboxTemplate`。
5. 调用 OpenAPI 成功后允许 CR 更新。

更新语义：

- OpenAPI `UpdateTemplateParam` 以一级属性为最小覆盖单位。
- CR 到 OpenAPI 转换时，传入某个一级对象必须传完整对象。
- 未变更字段不传，避免覆盖平台侧已有配置。
- `category=Public` 时清空 Private 专用字段。
- Public 转 Private 时，必须补齐 Private 必填字段。

#### DELETE

流程：

1. 跳过 Operator ServiceAccount 请求。
2. 如果 `spec.deletionPolicy=Retain`，直接允许删除 CR。
3. 如果 `status.templateID` 存在，调用 `DeleteSandboxTemplate(TemplateId)`。
4. 如果 `status.templateID` 不存在，按 `metadata.name` 查询模板并删除。
5. OpenAPI 返回不可删除时拒绝删除，用户需要先删除实例或改为 `Retain`。

### 6.4 SandboxWebhook

#### CREATE

流程：

1. 跳过 Operator ServiceAccount 请求。
2. 计算有效沙箱名：如果 `spec.name` 为空则使用 `metadata.name`；校验同 Namespace 下不存在其他 `Sandbox` 的有效沙箱名相同的对象。Validating Webhook 不修改对象本身。
3. 校验 `templateRef.id` 与 `templateRef.name` 二选一。
4. 若使用 `templateRef.name`，读取同 Namespace `SandboxTemplate.status.templateID`。
5. 读取实例级 KS3/KPFS Secret。
6. 转换为 `StartSandboxInstance` 请求，`spec.name` 不参与 OpenAPI 参数映射。
7. 调用 OpenAPI。
8. 记录返回 `InstanceId`、`Endpoint` 到 OperationRecord，不记录 Token。
9. 允许 CR 写入。

`Token` 直接写入 `status.token`。Poller 在绑定 `status.sandboxID` 后调用 `GetSandboxInstanceToken` 获取 token 并更新 status，不创建单独 Secret。

#### UPDATE

当前 OpenAPI 只支持 `UpdateSandboxInstance` 更新 `Timeout`，暂停/恢复接口未启用。因此 v1alpha1 支持的更新动作：

- `spec.timeoutSeconds` 变更 -> 调用 `UpdateSandboxInstance(InstanceId, Timeout)`。
- `spec.name` 变更 -> 只更新 Kubernetes CR，不调用 OpenAPI；Webhook 只校验同 Namespace 下唯一。
- 其他创建参数建议定义为不可变，避免用户误以为可热更新。

#### DELETE

流程：

1. `deletionPolicy=Retain` 时直接允许删除 CR。
2. 否则解析 `status.sandboxID`，调用 `DeleteSandboxInstance(InstanceIds=[id])`。
3. 平台侧已不存在时视为成功。

### 6.5 SandboxClaimWebhook

#### CREATE

流程：

1. 校验 `replicas`。
2. 解析 `templateRef`。
3. 循环调用 `StartSandboxInstance`，每个实例记录 `InstanceId`。
4. 任一创建失败时：
   - 默认拒绝整个 Claim 创建。
   - 对已创建成功的实例执行补偿删除，减少孤儿实例。
   - 如果补偿失败，记录 Event 和 OperationRecord，交由 Poller 后续清理。
5. 全部成功后允许 CR 写入。

命名规则：

- Claim `demo-claim` 的第 N 个实例对应本地 Sandbox CR 名称 `demo-claim-N`。
- 如果名称已存在则拒绝创建 Claim，避免覆盖用户资源。

#### UPDATE

建议 v1alpha1 只允许以下更新：

- `replicas` 增大：追加调用 `StartSandboxInstance`。
- `replicas` 减小：按序删除多余实例。
- `timeoutSeconds`、`env`、挂载配置对已创建实例不做批量热更新，除非后续 OpenAPI 支持完整更新。

如果要降低实现复杂度，v1alpha1 可以把 `spec` 定义为创建后不可变，仅允许删除重建。

#### DELETE

- `deletionPolicy=Delete`：删除 Claim 管理的所有 Sandbox 和平台实例。
- `deletionPolicy=Retain`：只删除 Claim，保留 Sandbox CR 和平台实例，并清理其 `ownerReferences` 或 `claimRef` 状态。

## 7. Reconciler 与 Poller 设计

### 7.1 Reconcile 策略

普通 Watch 触发的 Reconcile 不执行平台写操作：

```go
func (r *SandboxTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    return ctrl.Result{}, nil
}
```

允许的本地动作：

- 为新对象补 finalizer。
- 清理 OperationRecord。
- 聚合 Claim 下 Sandbox 状态。
- 当 status 缺失但 OperationRecord 存在时，触发一次快速同步。

### 7.2 Poller 主循环

```go
func (p *Poller) Run(ctx context.Context) {
    ticker := time.NewTicker(p.Interval)
    for {
        select {
        case <-ticker.C:
            p.syncAllNamespaces(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

同步粒度：

1. 枚举启用了 `sandbox-openapi-credentials` 的 Namespace。
2. 对每个 Namespace 获取 OpenAPI 凭据。
3. 调用 `GetSandboxTemplateList(PageNum, PageSize)` 分页拉取模板。
4. 调用 `GetSandboxInstanceList(PageNum, PageSize)` 分页拉取实例。
5. 更新已有 CR `status`，并把 OpenAPI 侧可反推的非敏感元数据反写到 CR `spec`。
6. 对平台存在但 CR 不存在的资源，根据配置决定是否创建影子 CR。

建议配置：

| 配置项 | 默认值 | 说明 |
| --- | --- | --- |
| `pollInterval` | `30s` | 常规同步周期。 |
| `fastPollInterval` | `5s` | Webhook 成功后短时间快速同步。 |
| `pageSize` | `100` | OpenAPI List 分页大小。 |
| `maxConcurrentNamespaces` | `5` | 并发同步 Namespace 数。 |
| `adoptExternalResources` | `true` | 是否把平台侧外部创建资源补为 CR。 |

### 7.3 SandboxTemplate 同步

输入：`GetSandboxTemplateList` 和必要时 `GetSandboxTemplate(TemplateId)`。

处理：

- 通过 `status.templateID` 优先匹配本地 CR。
- 若无 ID，则通过 `metadata.name == TemplateName` 匹配。
- 若仍无匹配且开启 adoption，则创建 `SandboxTemplate` CR：
  - `metadata.name` 使用平台模板名规范化。
  - `spec` 尽量由详情反推。
  - 增加 label `sandbox.kce.ksyun.com/adopted: "true"`。
- 将 OpenAPI 返回的模板元数据反写到 `spec`，包括描述、类型、镜像元数据、命令、端口、资源、环境变量、网络、预热和配额等可安全反推字段。
- 不反写 KS3、KPFS、镜像仓库、Klog 等凭据 Secret；如果 OpenAPI 返回的平台侧凭据摘要与当前 `credentialRef` 指向的 Secret 不一致，则更新 `status.credentialDrift` 和 `CredentialDrift` Condition。
- 更新 `status.phase/rawStatus/externalUpdatedAt/canDelete/quota/preheat/klog/createdAt/updatedAt/conditions`。

### 7.4 Sandbox 同步

输入：`GetSandboxInstanceList` 和必要时 `GetSandboxInstance(InstanceId)`。

处理：

- 通过 `status.sandboxID` 匹配本地 Sandbox。
- 通过 OperationRecord 的 `sandboxIDs` 绑定 Claim 创建出的实例。
- 若实例存在但 CR 不存在且开启 adoption，则创建 `Sandbox` CR。
- 将 OpenAPI 返回的实例元数据反写到 `spec`，当前主要包括 `spec.timeoutSeconds`；`spec.name` 仅为 Kubernetes 侧标识，OpenAPI 无对应字段，反写时保持已有值，缺失时使用 `metadata.name`。
- 不反写实例级 KS3/KPFS 凭据 Secret；如果能识别平台侧凭据漂移，则只更新 `status.credentialDrift` 和 Condition。
- 更新 `status.phase/rawStatus/externalUpdatedAt/template/timeout/createTime/endTime/customConfiguration/conditions`。
- 对 Running 实例调用 `GetSandboxInstanceToken`，把 token 写入 `status.token`。

### 7.5 SandboxClaim 同步

Claim 本身不是 OpenAPI 资源，由 Operator 聚合其关联 Sandbox：

- `desired = spec.replicas`
- `created = len(status.sandboxes)`
- `ready = phase == Running 的数量`
- `failed = phase == Failed/Unhealthy 的数量`

当所有关联 Sandbox 处于 Running：

- `phase=Successful`
- `Ready=True`
- `Bound=True`

当任一实例失败：

- `phase=Failed`
- `Ready=False`
- 记录失败实例名和 OpenAPI 状态。

### 7.6 Spec 反写与凭据漂移

OpenAPI 是权威源，控制台、SDK 或直接调用 OpenAPI 修改资源后，Poller 需要把外部变更同步回 Kubernetes CR。

同步策略：

- 模板元数据可反写 `spec`：描述、类型、镜像元数据、命令、端口、资源、环境变量、网络、预热、配额等字段。
- 沙箱实例元数据可反写 `spec`：当前主要是 `timeoutSeconds`。暂停/恢复接口未开放，v1alpha1 不处理 `spec.paused`。
- 不做 Kubernetes 侧冲突检测。若控制台更新后用户又修改 CR，则下一次 Webhook 写 OpenAPI 直接覆盖平台值；若用户修改后控制台再更新，则 Poller 再反写 CR。
- 每次从 OpenAPI 观察到资源更新时间时写入 `status.externalUpdatedAt`，用于判断最近一次平台侧更新时间。
- Operator 反写 `spec` 时必须使用 Operator ServiceAccount，并在 Webhook 中跳过 OpenAPI 写操作，避免同步循环。

凭据同步策略：

- 不反向修改用户 Secret。
- 除 Sandbox 访问 token 按产品要求写入 `status.token` 外，不把 AK/SK、密码等敏感凭据写入 CR `spec` 或 `status`。
- 若 OpenAPI 返回脱敏 AccessKey、凭据摘要或更新时间，Operator 只写入 `status.credentialDrift`。
- 若能判断平台侧凭据与当前 Namespace Secret 不一致，设置 `CredentialDrift=True`；若 OpenAPI 无法提供足够信息，则设置 `inSync=unknown`。
- 用户后续修改 CR 或 Secret 时，Webhook 读取当前 Secret 并调用 OpenAPI，以 Kubernetes 侧 Secret 覆盖平台侧凭据。

## 8. OpenAPI Client 设计

包建议：

```text
internal/openapi
  client.go
  request.go
  response.go
  template.go
  sandbox.go
  errors.go
```

接口：

```go
type Interface interface {
    CreateTemplate(ctx context.Context, req CreateTemplateRequest) (*CreateTemplateResponse, error)
    UpdateTemplate(ctx context.Context, req UpdateTemplateRequest) (*UpdateTemplateResponse, error)
    DeleteTemplate(ctx context.Context, templateID string) error
    GetTemplate(ctx context.Context, templateID string) (*Template, error)
    ListTemplates(ctx context.Context, req ListTemplatesRequest) (*TemplateList, error)

    StartSandbox(ctx context.Context, req StartSandboxRequest) (*StartSandboxResponse, error)
    UpdateSandbox(ctx context.Context, req UpdateSandboxRequest) error
    DeleteSandbox(ctx context.Context, instanceIDs []string) error
    GetSandbox(ctx context.Context, instanceID string) (*Sandbox, error)
    ListSandboxes(ctx context.Context, req ListSandboxesRequest) (*SandboxList, error)
    GetSandboxToken(ctx context.Context, instanceID string) (*SandboxToken, error)
}
```

响应模型注意：

- Java OpenAPI `BaseResponse` 返回字段为 `RequestId` 和 `Data`。
- Go client 需要先解析公共响应，再把 `Data` 反序列化到具体结构。
- OpenAPI 错误需要保留 HTTP 状态码、业务错误码、`RequestId` 和原始响应体摘要。

重试策略：

| 错误 | 策略 |
| --- | --- |
| 网络超时、连接重置 | 指数退避重试，最多 3 次。 |
| HTTP 429/5xx | 指数退避重试。 |
| 参数错误、鉴权失败、配额不足 | 不重试，直接返回 admission 拒绝。 |
| 查询资源不存在 | 读路径视为已删除，写路径按语义处理。 |

## 9. 字段映射细节

### 9.1 模板创建

| CR | OpenAPI `CreateTemplateParam` |
| --- | --- |
| `metadata.name` | `TemplateName` |
| `spec.description` | `Description` |
| `spec.category` | `TemplateCategory` |
| `spec.type` | `TemplateType` |
| `spec.image.*` | `ImageConfig.*` |
| `spec.ports` | `Ports` |
| `spec.command` | `Command` |
| `spec.kec.*` | `KecConfig.*` |
| `spec.resources.cpu` | `Cpu` |
| `spec.resources.memoryGB` | `Memory` |
| `spec.env[]` | `Envs[]` |
| `spec.skill.*` | `SkillConfig.*` |
| `spec.network.*` | `NetworkConfig.*` |
| `spec.klog.enabled` | `KlogConfig.KlogEnable` |
| `spec.kpfsMountConfig.*` | `KpfsMountConfig.*` |
| `spec.ks3MountConfig.*` | `Ks3MountConfig.*` |
| KS3/KPFS Secret | `AccessKey`、`SecretAccessKey` |
| `spec.preheat.*` | `PreheatConfig.*` |
| `spec.instanceQuota` | `InstanceQuota` |

### 9.2 实例启动

| CR | OpenAPI `StartSandboxInstanceParam` |
| --- | --- |
| `spec.name` | 无，仅 Kubernetes 侧名称标识，不传 OpenAPI |
| `spec.templateRef.id/name` | `TemplateId` |
| `spec.timeoutSeconds` | `Timeout` |
| `spec.ks3MountConfig.*` | `Ks3MountConfig.*` |
| `spec.kpfsMountConfig.*` | `KpfsMountConfig.*` |
| KS3/KPFS Secret | `AccessKey`、`SecretAccessKey` |
| `spec.env[]` | `Envs[]` |

### 9.3 状态同步

| OpenAPI 模板响应 | CR Status |
| --- | --- |
| `TemplateId` | `status.templateID` |
| `Status` | `status.rawStatus`、`status.phase` |
| `CanDelete` | `status.canDelete` |
| `CreatedAt` | `status.createdAt` |
| `UpdatedAt` | `status.updatedAt`、`status.externalUpdatedAt` |
| `KlogConfig.KlogProjectName` | `status.klog.projectName` |
| `KlogConfig.KlogPoolName` | `status.klog.poolName` |
| `PreheatConfig.PreheatedInstanceNumber` | `status.preheat.preheatedInstanceNumber` |
| `InstanceQuota` | `status.quota.instanceQuota` |
| `RemainingInstanceQuota` | `status.quota.remainingInstanceQuota` |
| `RemainingSystemInstanceQuota` | `status.quota.remainingSystemInstanceQuota` |

| OpenAPI 实例响应 | CR Status |
| --- | --- |
| `InstanceId` | `status.sandboxID` |
| `TemplateId` | `status.template.id` |
| `TemplateType` | `status.template.type` |
| `TemplateCategory` | `status.template.category` |
| `Status` | `status.rawStatus`、`status.phase` |
| 平台侧更新时间 | `status.externalUpdatedAt` |
| `Timeout` | `status.timeoutSeconds` |
| `CreateTime` | `status.createTime` |
| `EndTime` | `status.endTime` |
| `CustomConfiguration` | `status.customConfiguration` |
| `GetSandboxInstanceToken.Token` | `status.token` |

### 9.4 Spec 反写映射

| OpenAPI 模板响应 | 反写 CR Spec |
| --- | --- |
| `Description` | `spec.description` |
| `TemplateCategory` | `spec.category` |
| `TemplateType` | `spec.type` |
| `ImageConfig` | `spec.image` 中非敏感字段 |
| `Command` | `spec.command` |
| `Ports` | `spec.ports` |
| `Cpu`、`Memory` | `spec.resources` |
| `Envs` | `spec.env` |
| `NetworkConfig` | `spec.network` |
| `PreheatConfig` | `spec.preheat` |
| `InstanceQuota` | `spec.instanceQuota` |

| OpenAPI 实例响应 | 反写 CR Spec |
| --- | --- |
| `Timeout` | `spec.timeoutSeconds` |
| 无 | `spec.name` 保持不变；缺失时默认 `metadata.name` |

凭据类字段不做 spec/Secret 反写。KS3、KPFS、镜像仓库、Klog 等凭据只允许写入 `status.credentialDrift` 中的脱敏摘要和同步状态。

## 10. 一致性、幂等与补偿

### 10.1 幂等键

当前 OpenAPI 参数未看到显式 idempotency token。Operator 侧需要用以下方式降低重复创建风险：

- 模板以 `metadata.name` 作为唯一业务键。
- SandboxClaim 创建出的 Sandbox 使用确定性名称 `${claimName}-${index}`。
- Webhook 在 CREATE 前查询同名模板或同名本地 Sandbox。
- OpenAPI 调用成功后写 OperationRecord，记录平台返回 ID。
- Poller 优先用 OperationRecord 绑定 status。

### 10.2 失败窗口

Webhook 调用 OpenAPI 成功，但 APIServer 后续写 CR 失败，会产生平台侧孤儿资源。这是“在 admission 中执行外部写操作”的天然风险。

缓解措施：

1. OpenAPI 创建前尽量做 Kubernetes 侧冲突检查。
2. OpenAPI 资源命名带 CR 名称，便于后续人工或自动识别。
3. Poller 开启 adoption，把平台侧孤儿资源补为 CR。
4. 对 Claim 批量创建失败执行补偿删除。
5. 记录 OperationRecord 和 Event，便于追踪。

### 10.3 删除一致性

删除流程优先通过 Webhook 调用 OpenAPI。若 Webhook 成功但 CR 删除失败，下次删除会再次调用 OpenAPI，应将“平台资源不存在”视为成功。

如果 Webhook 不可用，`failurePolicy=Fail` 会拒绝用户删除，避免平台资源和 CR 分离。

## 11. RBAC

Operator ServiceAccount 需要：

- 管理 `sandboxtemplates`、`sandboxclaims`、`sandboxes`。
- 更新上述资源 `/status`。
- 更新 finalizers。
- 读取业务 Namespace Secret。
- 读取 ConfigMap，用于 env `valueFrom`。
- 创建 Event。
- 使用 Lease 做 leader election。

Secret 权限较敏感。生产建议使用以下约束：

- 只监听明确配置的 Namespace。
- 或通过 `RoleBinding` 下发到指定业务 Namespace，避免全局 Secret 读权限。

## 12. 可观测性

指标：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `sandbox_operator_openapi_requests_total` | counter | OpenAPI 请求总数，按 action/status 分类。 |
| `sandbox_operator_openapi_request_duration_seconds` | histogram | OpenAPI 请求耗时。 |
| `sandbox_operator_sync_duration_seconds` | histogram | 单 Namespace 同步耗时。 |
| `sandbox_operator_sync_errors_total` | counter | 同步失败次数。 |
| `sandbox_operator_webhook_rejections_total` | counter | Webhook 拒绝次数。 |
| `sandbox_operator_managed_templates` | gauge | 当前管理模板数。 |
| `sandbox_operator_managed_sandboxes` | gauge | 当前管理实例数。 |

日志字段：

- `namespace`
- `name`
- `kind`
- `action`
- `openapiAction`
- `requestId`
- `templateID`
- `sandboxID`
- `generation`

Event：

- `OpenAPICreateSucceeded`
- `OpenAPICreateFailed`
- `OpenAPIUpdateSucceeded`
- `OpenAPIUpdateFailed`
- `OpenAPIDeleteSucceeded`
- `OpenAPIDeleteFailed`
- `OpenAPISynced`
- `CredentialInvalid`

## 13. 安全设计

1. Webhook 服务必须启用 TLS，并由 cert-manager 或安装脚本注入 `caBundle`。
2. OpenAPI AK/SK 只从 Secret 读取，不写日志、不写 status。
3. Token 写入 `Sandbox.status.token`，不额外创建 Secret。
4. Operator ServiceAccount 需要最小权限部署。
5. 业务用户不能使用 Operator ServiceAccount。
6. Webhook 错误信息需要脱敏，不返回 Secret 内容。
7. 对外部创建资源的 adoption 应可关闭，避免误接管不属于该集群的资源。

## 14. 项目结构建议

```text
sandbox-operator
├── api
│   └── v1alpha1
│       ├── sandboxtemplate_types.go
│       ├── sandboxclaim_types.go
│       ├── sandbox_types.go
│       ├── condition.go
│       └── zz_generated.deepcopy.go
├── internal
│   ├── controller
│   │   ├── sandboxtemplate_controller.go
│   │   ├── sandboxclaim_controller.go
│   │   ├── sandbox_controller.go
│   │   └── poller.go
│   ├── webhook
│   │   ├── sandboxtemplate_webhook.go
│   │   ├── sandboxclaim_webhook.go
│   │   ├── sandbox_webhook.go
│   │   └── admission.go
│   ├── openapi
│   │   ├── client.go
│   │   ├── template.go
│   │   ├── sandbox.go
│   │   └── errors.go
│   ├── credentials
│   │   └── manager.go
│   ├── mapper
│   │   ├── template_mapper.go
│   │   ├── sandbox_mapper.go
│   │   └── status_mapper.go
│   └── status
│       └── conditions.go
├── config
│   ├── crd
│   ├── rbac
│   ├── webhook
│   ├── manager
│   └── default
├── cmd
│   └── main.go
└── docs
```

## 15. 实施计划

### 阶段一：项目脚手架与 CRD

- 初始化 kubebuilder/controller-runtime 结构。
- 定义 `SandboxTemplate`、`SandboxClaim`、`Sandbox` Go types。
- 生成 CRD、RBAC、Webhook manifests。
- 增加基础单元测试，校验 DeepCopy、CRD schema 和枚举。

### 阶段二：OpenAPI Client 与字段映射

- 实现 Action 请求封装。
- 实现模板和实例 client。
- 实现 Secret 凭据读取。
- 实现 CR <-> OpenAPI mapper。
- 用 fake HTTP server 覆盖成功、业务错误、超时、分页。

### 阶段三：Webhook 写路径

- 实现三个 Webhook。
- 实现 Operator ServiceAccount 跳过逻辑。
- 实现模板创建、更新、删除。
- 实现实例创建、更新 timeout、删除。
- 实现 Claim 批量创建和失败补偿。

### 阶段四：Poller 与 Status 同步

- 实现 Namespace discovery。
- 实现模板分页同步。
- 实现实例分页同步。
- 实现 Claim 聚合状态。
- 实现 `status.token` 写入。
- 实现 adoption 配置。

### 阶段五：可靠性与生产化

- 增加 metrics、Event、结构化日志。
- 增加 leader election。
- 增加限速队列和 OpenAPI 并发控制。
- 增加 e2e 测试：创建、更新、删除、外部资源同步、Webhook 不可用。
- 输出部署文档和示例 YAML。

## 16. 测试方案

单元测试：

- 字段映射：Template/Sandbox/Claim 到 OpenAPI 请求。
- 状态映射：OpenAPI 状态到 CR phase/conditions。
- Secret 解析：OpenAPI、dockerconfigjson、KS3、KPFS。
- Webhook 校验：非法字段、缺凭据、不可变字段、删除策略。
- OpenAPI Client：错误码、重试、分页。

集成测试：

- 使用 envtest 启动 APIServer 和 Webhook。
- 使用 fake OpenAPI server 模拟 Action 响应。
- 验证 CREATE 成功后 CR 入库、Poller 回填 status。
- 验证 OpenAPI 失败时 admission 拒绝。
- 验证 Operator ServiceAccount 更新 status 不触发 OpenAPI。
- 验证控制台/OpenAPI 修改模板元数据后，Poller 反写 CR `spec` 和 `status.externalUpdatedAt`。
- 验证控制台/OpenAPI 修改凭据后，Operator 不修改 Secret，只更新 `status.credentialDrift` 和 Condition。

E2E 测试：

- 创建 `SandboxTemplate`，等待 `status.phase=Ready`。
- 创建 `SandboxClaim(replicas=3)`，等待 3 个 Sandbox Running。
- 手动创建 `Sandbox`，等待 `status.token` 生成。
- 控制台更新 Sandbox timeout 后，确认 CR `spec.timeoutSeconds` 和 `status.externalUpdatedAt` 被同步。
- 删除 Sandbox，确认平台实例删除。
- 删除 Template 且模板下存在实例，确认 OpenAPI 拒绝并保留 CR。
- 控制台创建模板后，确认 Operator adoption 为 CR。

## 17. 风险与待确认事项

| 风险 | 影响 | 建议 |
| --- | --- | --- |
| Validating Webhook 不能回填 ID | 需要靠 Poller 或 OperationRecord 绑定平台资源 | 实现 OperationRecord，并强制命名唯一。 |
| OpenAPI 缺少显式幂等 token | APIServer 重试或网络抖动可能导致重复创建 | 创建前查询、确定性命名、失败补偿。 |
| 当前暂停/恢复 OpenAPI 未启用 | 不能可靠支持 `spec.paused` | v1alpha1 不暴露暂停/恢复，后续接口恢复再加。 |
| 外部平台资源与 CR 同名 | adoption 可能误绑定 | 同 Namespace/账号/地域范围内强制唯一，adoption 可关闭。 |
| 平台侧凭据被控制台修改 | Kubernetes Secret 与平台实际凭据不一致 | 不反写 Secret，只写 `status.credentialDrift` 并提示用户更新 Secret。 |
| `Sandbox.spec.name` 与 `metadata.name` 不一致 | 用户可能误以为底层平台有沙箱名称 | 文档和 CRD 注释明确该字段只用于 Kubernetes 侧标识，Webhook 保证 Namespace 内唯一。 |
| Secret 权限过大 | 安全风险 | 优先 Namespace 级 RoleBinding，限制 watched namespaces。 |
| Claim 批量创建部分成功 | 容易产生孤儿实例 | 失败补偿删除，保留 OperationRecord。 |

## 18. 推荐结论

推荐按照“Webhook 负责用户写、Poller 负责从 OpenAPI 读并同步 CR”的架构实现，保持与现有 PDF 设计一致。OpenAPI 是权威源，Poller 需要同步 `status`，也需要把控制台/OpenAPI 修改的非敏感元数据反写到 `spec`。但必须在工程实现中补齐以下约束，否则会出现资源 ID 丢失、重复创建或凭据所有权混乱问题：

1. `metadata.name` 必须作为模板唯一业务键。
2. 引入 OperationRecord 记录 Webhook 调用 OpenAPI 后返回的 `TemplateId/InstanceId`。
3. Poller 必须优先消费 OperationRecord，再使用 List/Get 结果更新 CR `status` 和可反推的 `spec`。
4. `Sandbox.spec.name` 仅作为 Kubernetes 侧沙箱名，允许修改，不传 OpenAPI，并由 Webhook 保证同 Namespace 唯一。
5. `SandboxClaim` 创建出的 `Sandbox` 使用确定性名称。
6. 控制台修改凭据时不反写 Secret，只写 `status.credentialDrift`。
7. `spec.paused` 暂不进入 v1alpha1，等待 OpenAPI 暂停/恢复接口稳定后再支持。
