# CR 示例

本文给出常用 Secret 与 CR 示例。示例中的 `sandbox-demo`、镜像地址、VPC、Bucket、文件系统、AK/SK 等值需要替换为实际环境值。

## 1. 凭据 Secret 示例

### 1.1 OpenAPI 凭据

OpenAPI 凭据用于 operator 调用沙箱 OpenAPI。默认 Secret 名称为 `sandbox-openapi-credentials`，必须与要管理的 CR 位于同一个业务命名空间。

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sandbox-openapi-credentials
  namespace: sandbox-demo
type: Opaque
stringData:
  # OpenAPI AccessKey ID。
  accessKeyId: "<OPENAPI_ACCESS_KEY_ID>"

  # OpenAPI SecretAccessKey。
  secretAccessKey: "<OPENAPI_SECRET_ACCESS_KEY>"

  # 账号 ID。建议填写，便于和命名空间对应。
  accountId: "<ACCOUNT_ID>"

  # OpenAPI 区域，例如 cn-beijing-6。
  region: "cn-beijing-6"
```

### 1.2 KS3/KPFS 挂载凭据

模板或实例配置 `ks3MountConfig`、`kpfsMountConfig` 时，需要通过 `storageCredentialRef` 引用该 Secret。

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-credential
  namespace: sandbox-demo
type: Opaque
stringData:
  # KS3/KPFS AccessKey。
  accessKey: "<STORAGE_ACCESS_KEY_ID>"

  # KS3/KPFS SecretAccessKey。
  secretAccessKey: "<STORAGE_SECRET_ACCESS_KEY>"
```

### 1.3 镜像仓库凭据

私有镜像需要通过 `registryCredentialRef` 引用镜像仓库 Secret。可以使用标准 `docker-registry` Secret。

```bash
kubectl create secret docker-registry image-credential \
  -n sandbox-demo \
  --docker-server=hub-vpc-cn-beijing-6.kce.ksyun.com \
  --docker-username='<REGISTRY_USERNAME>' \
  --docker-password='<REGISTRY_PASSWORD>'
```

等价 YAML：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: image-credential
  namespace: sandbox-demo
type: kubernetes.io/dockerconfigjson
stringData:
  .dockerconfigjson: |
    {
      "auths": {
        "hub-vpc-cn-beijing-6.kce.ksyun.com": {
          "username": "<REGISTRY_USERNAME>",
          "password": "<REGISTRY_PASSWORD>"
        }
      }
    }
```

## 2. SandboxTemplate 完整示例

该示例覆盖模板创建和更新中常用字段。`spec.template.spec.env` 使用 `name/value`，与 Kubernetes Pod EnvVar 风格一致。

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxTemplate
metadata:
  # Kubernetes CR 名称，同时作为 OpenAPI 模板名。
  # 如果由 OpenAPI 同步回集群，合法模板名会作为 CR 名称；否则使用模板 ID。
  name: full-template
  namespace: sandbox-demo
  labels:
    owner: platform
spec:
  # 可选。不填写时使用同命名空间的 sandbox-openapi-credentials。
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # 对应 OpenAPI 模板描述。
  description: "full template created from CR"

  # 模板类型。支持 AIO / Browser / Code / Custom，也兼容小写输入。
  type: Custom

  # 访问类型。Private 支持预热池；Public 不支持 pool。
  access: Private

  template:
    spec:
      image:
        # 镜像来源：Public / Personal / Enterprise。
        source: Personal

        # 完整镜像地址。
        image: "hub-vpc-cn-beijing-6.kce.ksyun.com/sandbox/aio-skill:v20260528"

        # 私有镜像仓库凭据。公共镜像可以删除该字段。
        registryCredentialRef:
          name: image-credential
          server: hub-vpc-cn-beijing-6.kce.ksyun.com

      # KEC 机型、系统盘和数据盘配置。
      # cpu/memory 是全局资源字段。
      # 推荐使用 instanceSpecs 配置一个或多个可选机型。
      # instanceSpecs 中只配置机型、系统盘和数据盘。
      kecConfig:
        cpu: "2"
        memory: 4Gi
        instanceSpecs:
          - instanceType: S6.2B
            systemDisk:
              type: SSD3.0
              size: 20Gi
            dataDisks:
              - name: data-0
                type: SSD3.0
                size: 50Gi
                deleteWithInstance: true
                path: /data
                fsType: ext4
          - instanceType: S6.2A
            systemDisk:
              type: SSD3.0
              size: 20Gi
            dataDisks:
              - name: data-0
                type: SSD3.0
                size: 50Gi
                deleteWithInstance: true
                path: /data
                fsType: ext4

      # 容器端口。
      ports:
        - name: api
          containerPort: 8080
          protocol: TCP
        - name: tty
          containerPort: 7681
          protocol: TCP
        - name: chrome
          containerPort: 9222
          protocol: TCP
        - name: vscode
          containerPort: 49999
          protocol: TCP

      # 启动命令。
      startCommand: /usr/local/bin/aio-vscode-entrypoint.sh

      # 模板环境变量。注意这里是 name/value。
      env:
        - name: APP_ENV
          value: prod
        - name: TEST_KEY
          value: ghw

      # KS3/KPFS 挂载凭据。只要启用挂载，就需要引用同命名空间 Secret。
      storageCredentialRef:
        name: storage-credential

      # KS3 挂载。删除挂载时可以删除该字段，或设置 enabled: false。
      ks3MountConfig:
        enabled: true
        mountPoints:
          - bucketName: ghw-test-bj-7583
            remotePath: /
            localMountPath: /mnt2
            readOnly: true

      # KPFS 挂载。删除挂载时可以删除该字段，或设置 enabled: false。
      kpfsMountConfig:
        enabled: true
        mountPoints:
          - fileSystemName: sandbox-test
            remotePath: /
            localMountPath: /mnt1
            readOnly: true

      # 网络配置。
      # 省略 networkConfig 或配置为 {} 时，Operator 默认只启用公网访问。
      networkConfig:
        enablePublic: true
        enablePrivate: true
        cidrBlock: "10.0.2.0/24"
        sharedInternetAccessEnable: true
        userVpcId: 3c9d8253-778e-468a-b18d-6670c5204904
        userSgId: 4fc47f84-c5e1-43df-8b7a-5e838f18f793
        userSubnetId: a7cf00fb-87cd-4615-8d11-af131472245e
        availabilityZone: cn-beijing-6a

      # 技能配置。
      skillConfig:
        enable: true
        spaceIds:
          - space-abc123
          - space-def456
        enablePublicSkill: false

      # Private 模板预热池目标大小。
      pool:
        targetSize: 1

      # 日志配置。当前 CR 到 OpenAPI 创建/更新主要使用 enabled；
      # 从 OpenAPI 同步回来时可能看到 projectName、containerPoolName 等字段。
      observability:
        logging:
          enabled: true
```

创建后，operator 会补充平台模板 ID 注解，并由 reconciler 回写状态：

```yaml
metadata:
  annotations:
    sandbox.kce.ksyun.com/template-id: "<TEMPLATE_ID>"
status:
  phase: Ready
  canDelete: true
  externalUpdatedAt: "2026-07-06T08:30:55Z"
  preheat:
    enabled: true
    number: 1
    preheatedInstanceNumber: 1
```

## 3. Public SandboxTemplate 示例

`access: Public` 的模板不能配置 `spec.template.spec.pool`，同步回来时也不会出现 `status.preheat`。

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxTemplate
metadata:
  name: public-browser
  namespace: sandbox-demo
spec:
  description: "public browser runtime"
  type: Browser
  access: Public
  template:
    spec:
      image:
        source: Public
        image: "hub.kce.ksyun.com/sandbox/browser:v20260608"
      kecConfig:
        cpu: "2"
        memory: 4Gi
      ports:
        - name: browser
          containerPort: 8080
          protocol: TCP
```

## 4. Sandbox 完整示例

该示例基于已有模板创建单个沙箱实例。`Sandbox` 的环境变量使用 `key/value`，与 `SandboxTemplate` 的 `name/value` 不同。

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: Sandbox
metadata:
  # Kubernetes CR 名称就是实例在集群内的名称标识。
  # 不要配置 spec.name；该字段当前不支持。
  name: full-sandbox
  namespace: sandbox-demo
spec:
  # 可选。不填写时使用同命名空间的 sandbox-openapi-credentials。
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # 引用模板。name 和 id 二选一。
  templateRef:
    # 引用同命名空间 SandboxTemplate。
    name: full-template

    # 或直接引用平台模板 ID。
    # id: "<TEMPLATE_ID>"

  # 实例超时时间，单位秒。当前 Sandbox 更新只支持修改该字段。
  timeoutSeconds: 3600

  # 实例环境变量。注意这里是 key/value。
  env:
    - key: RUNTIME_ENV
      value: single

  # 实例级 KS3/KPFS 挂载凭据。
  storageCredentialRef:
    name: storage-credential

  # 实例级 KS3 挂载，会覆盖模板中的 KS3 挂载。
  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true

  # 实例级 KPFS 挂载，会覆盖模板中的 KPFS 挂载。
  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true
```

创建后，operator 会补充平台实例 ID、模板 ID、endpoint、token 等注解，并回写状态：

```yaml
metadata:
  annotations:
    sandbox.kce.ksyun.com/sandbox-id: "<SANDBOX_ID>"
    sandbox.kce.ksyun.com/template-id: "<TEMPLATE_ID>"
    sandbox.kce.ksyun.com/endpoint: "<ENDPOINT>"
    sandbox.kce.ksyun.com/token: "<TOKEN>"
status:
  phase: Running
  timeoutSeconds: 3600
  endpoint: "<ENDPOINT>"
  token: "<TOKEN>"
  imageUrl: "hub-vpc-cn-beijing-6.kce.ksyun.com/sandbox/aio-skill:v20260528"
  command: /usr/local/bin/aio-vscode-entrypoint.sh
  env:
    - key: RUNTIME_ENV
      value: single
  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true
  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true
```

## 5. Inline Template Sandbox 示例

内联模板适合在创建实例时临时创建一个模板。operator 会先调用 OpenAPI 创建模板，再启动实例。

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: Sandbox
metadata:
  name: inline-full-sandbox
  namespace: sandbox-demo
spec:
  timeoutSeconds: 3600

  template:
    # 可选。不填写时使用 "<sandbox-name>-inline-template"。
    name: inline-full-template
    description: "inline template created from Sandbox CR"
    type: Custom
    access: Private
    spec:
      image:
        source: Personal
        image: hub-vpc-cn-beijing-6.kce.ksyun.com/sandbox/aio-skill:v20260528
        registryCredentialRef:
          name: image-credential
          server: hub-vpc-cn-beijing-6.kce.ksyun.com

      kecConfig:
        cpu: "2"
        memory: 4Gi
        instanceSpecs:
          - instanceType: S6.2B
            systemDisk:
              type: SSD3.0
              size: 20Gi
            dataDisks:
              - name: data-0
                type: SSD3.0
                size: 50Gi
                deleteWithInstance: true
                path: /data
                fsType: ext4
          - instanceType: S6.2A
            systemDisk:
              type: SSD3.0
              size: 20Gi
            dataDisks:
              - name: data-0
                type: SSD3.0
                size: 50Gi
                deleteWithInstance: true
                path: /data
                fsType: ext4

      ports:
        - name: api
          containerPort: 8080
          protocol: TCP
        - name: vscode
          containerPort: 49999
          protocol: TCP

      startCommand: /usr/local/bin/aio-vscode-entrypoint.sh

      env:
        - name: APP_ENV
          value: prod

      storageCredentialRef:
        name: storage-credential

      kpfsMountConfig:
        enabled: true
        mountPoints:
          - fileSystemName: sandbox-test
            remotePath: /
            localMountPath: /mnt1
            readOnly: true

      ks3MountConfig:
        enabled: true
        mountPoints:
          - bucketName: ghw-test-bj-7583
            remotePath: /
            localMountPath: /mnt2
            readOnly: true

      networkConfig:
        # 省略 networkConfig 或配置为 {} 时，Operator 默认只启用公网访问。
        enablePublic: true
        enablePrivate: true
        cidrBlock: "10.0.2.0/24"
        sharedInternetAccessEnable: true
        userVpcId: 3c9d8253-778e-468a-b18d-6670c5204904
        userSgId: 4fc47f84-c5e1-43df-8b7a-5e838f18f793
        userSubnetId: a7cf00fb-87cd-4615-8d11-af131472245e
        availabilityZone: cn-beijing-6a

      pool:
        targetSize: 1

      observability:
        logging:
          enabled: true

  # 实例级字段会覆盖内联模板中的默认值。
  env:
    - key: RUNTIME_ENV
      value: single

  storageCredentialRef:
    name: storage-credential

  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true

  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true
```

## 6. SandboxClaim 完整示例

`SandboxClaim` 用于一次性批量创建实例。它不支持内联模板，只能通过 `templateRef` 引用已有模板。

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxClaim
metadata:
  # 子 Sandbox 会按 "<claim-name>-<index>" 命名，例如 full-claim-0。
  name: full-claim
  namespace: sandbox-demo
spec:
  # 可选。不填写时使用同命名空间的 sandbox-openapi-credentials。
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # 一次性创建实例数量，必须大于 0。
  replicas: 2

  # 引用模板。name 和 id 二选一。
  templateRef:
    name: full-template
    # id: "<TEMPLATE_ID>"

  # 每个实例的超时时间，单位秒。
  timeoutSeconds: 3600

  # 每个实例创建时传入的环境变量。注意这里是 key/value。
  env:
    - key: RUNTIME_ENV
      value: claim

  # 每个实例的挂载凭据。
  storageCredentialRef:
    name: storage-credential

  # 每个实例的 KS3 挂载。
  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true

  # 每个实例的 KPFS 挂载。
  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true
```

创建后，Claim 会进入 `Pending`、`Running`、`Successful` 或 `Failed` 等状态，并记录子实例：

```yaml
status:
  phase: Successful
  desired: 2
  created: 2
  ready: 2
  failed: 0
  sandboxes:
    - name: full-claim-0
      phase: Running
    - name: full-claim-1
      phase: Running
```

`SandboxClaim` 是一次性声明，不支持更新。删除 Claim 不会删除已经创建出的子 `Sandbox`；如需删除实例，需要删除子 `Sandbox`。
