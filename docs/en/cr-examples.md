# CR Examples

This page provides common Secret and CR examples. Replace placeholder values such as `sandbox-demo`, image addresses, VPC, bucket, file system, and access keys with values from your actual environment.

## 1. Credential Secret Examples

### 1.1 OpenAPI credentials

OpenAPI credentials are used by the operator to call the Sandbox OpenAPI. The default Secret name is `sandbox-openapi-credentials`, and it must reside in the same business namespace as the CRs it manages.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sandbox-openapi-credentials
  namespace: sandbox-demo
type: Opaque
stringData:
  # OpenAPI AccessKey ID.
  accessKeyId: "<OPENAPI_ACCESS_KEY_ID>"

  # OpenAPI SecretAccessKey.
  secretAccessKey: "<OPENAPI_SECRET_ACCESS_KEY>"

  # Account ID. Recommended for correlating with namespaces.
  accountId: "<ACCOUNT_ID>"

  # OpenAPI region, e.g. cn-beijing-6.
  region: "cn-beijing-6"
```

### 1.2 KS3/KPFS mount credentials

When `ks3MountConfig` or `kpfsMountConfig` is configured in a template or instance, reference this Secret through `storageCredentialRef`.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: storage-credential
  namespace: sandbox-demo
type: Opaque
stringData:
  # KS3/KPFS AccessKey.
  accessKey: "<STORAGE_ACCESS_KEY_ID>"

  # KS3/KPFS SecretAccessKey.
  secretAccessKey: "<STORAGE_SECRET_ACCESS_KEY>"
```

### 1.3 Image registry credentials

Private images require an image registry Secret referenced by `registryCredentialRef`. You can use a standard `docker-registry` Secret.

```bash
kubectl create secret docker-registry image-credential \
  -n sandbox-demo \
  --docker-server=hub-vpc-cn-beijing-6.kce.ksyun.com \
  --docker-username='<REGISTRY_USERNAME>' \
  --docker-password='<REGISTRY_PASSWORD>'
```

Equivalent YAML:

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

## 2. Full SandboxTemplate Example

This example covers the commonly used fields when creating or updating a template. `spec.template.spec.env` uses `name`/`value` in the Kubernetes Pod EnvVar style.

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxTemplate
metadata:
  # Kubernetes CR name, also used as the OpenAPI template name.
  # When synced from OpenAPI to the cluster, a valid template name becomes the CR name;
  # otherwise the CR uses the full template ID.
  name: full-template
  namespace: sandbox-demo
  labels:
    owner: platform
spec:
  # Optional. If omitted, the Secret named sandbox-openapi-credentials in the same namespace is used.
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # OpenAPI template description.
  description: "full template created from CR"

  # Template type. Supports AIO / Browser / Code / Custom; lowercase input is also accepted.
  type: Custom

  # Access type. Private supports preheat pools; Public does not.
  access: Private

  template:
    spec:
      image:
        # Image source: Public / Personal / Enterprise.
        source: Personal

        # Full image address.
        image: "hub-vpc-cn-beijing-6.kce.ksyun.com/sandbox/aio-skill:v20260528"

        # Private image registry credential. Remove this field for public images.
        registryCredentialRef:
          name: image-credential
          server: hub-vpc-cn-beijing-6.kce.ksyun.com

      # KEC instance type, system disk, and data disk configuration.
      # cpu/memory are global resource fields.
      # Use instanceSpecs to configure one or more candidate instance types.
      # instanceSpecs only contains instance type, system disk, and data disks.
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

      # Container ports.
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

      # Start command.
      startCommand: /usr/local/bin/aio-vscode-entrypoint.sh

      # Template environment variables. Note that these use name/value.
      env:
        - name: APP_ENV
          value: prod
        - name: TEST_KEY
          value: ghw

      # KS3/KPFS mount credential. Required whenever any mount is enabled.
      storageCredentialRef:
        name: storage-credential

      # KS3 mount. To remove, delete this field or set enabled: false.
      ks3MountConfig:
        enabled: true
        mountPoints:
          - bucketName: ghw-test-bj-7583
            remotePath: /
            localMountPath: /mnt2
            readOnly: true

      # KPFS mount. To remove, delete this field or set enabled: false.
      kpfsMountConfig:
        enabled: true
        mountPoints:
          - fileSystemName: sandbox-test
            remotePath: /
            localMountPath: /mnt1
            readOnly: true

      # Network configuration.
      # When networkConfig is omitted or set to {}, the operator enables public access only.
      networkConfig:
        enablePublic: true
        enablePrivate: true
        cidrBlock: "10.0.2.0/24"
        sharedInternetAccessEnable: true
        userVpcId: 3c9d8253-778e-468a-b18d-6670c5204904
        userSgId: 4fc47f84-c5e1-43df-8b7a-5e838f18f793
        userSubnetId: a7cf00fb-87cd-4615-8d11-af131472245e
        availabilityZone: cn-beijing-6a

      # Skill configuration.
      skillConfig:
        enable: true
        spaceIds:
          - space-abc123
          - space-def456
        enablePublicSkill: false

      # Private template preheat pool target size.
      pool:
        targetSize: 1

      # Logging configuration. For CR-to-OpenAPI create/update, only enabled is mainly used;
      # fields such as projectName and containerPoolName may appear when syncing from OpenAPI.
      observability:
        logging:
          enabled: true
```

After creation, the operator adds the platform template ID annotation and the reconciler writes back the status:

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

## 3. Public SandboxTemplate Example

A template with `access: Public` cannot configure `spec.template.spec.pool`, and `status.preheat` will not appear when synced from OpenAPI.

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

## 4. Full Sandbox Example

This example creates a single sandbox instance from an existing template. `Sandbox` environment variables use `key`/`value`, unlike the `name`/`value` used by `SandboxTemplate`.

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: Sandbox
metadata:
  # The Kubernetes CR name is the instance's identifier within the cluster.
  # Do not configure spec.name; it is not supported.
  name: full-sandbox
  namespace: sandbox-demo
spec:
  # Optional. If omitted, the Secret named sandbox-openapi-credentials in the same namespace is used.
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # Reference an existing template. Use either name or id.
  templateRef:
    # Reference a SandboxTemplate in the same namespace.
    name: full-template

    # Or reference the platform template ID directly.
    # id: "<TEMPLATE_ID>"

  # Instance timeout in seconds. Currently this is the only field that can be updated on Sandbox.
  timeoutSeconds: 3600

  # Instance environment variables. Note that these use key/value.
  env:
    - key: RUNTIME_ENV
      value: single

  # Instance-level KS3/KPFS mount credential.
  storageCredentialRef:
    name: storage-credential

  # Instance-level KS3 mount; overrides the KS3 mount in the template.
  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true

  # Instance-level KPFS mount; overrides the KPFS mount in the template.
  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true
```

After creation, the operator adds platform annotations for instance ID, template ID, endpoint, and token, and writes back the status:

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

## 5. Inline Template Sandbox Example

An inline template is useful when you want to create a temporary template while creating an instance. The operator first calls OpenAPI to create the template and then starts the instance.

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: Sandbox
metadata:
  name: inline-full-sandbox
  namespace: sandbox-demo
spec:
  timeoutSeconds: 3600

  template:
    # Optional. Defaults to "<sandbox-name>-inline-template".
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
        # When networkConfig is omitted or set to {}, the operator enables public access only.
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

  # Instance-level fields override defaults from the inline template.
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

## 6. Full SandboxClaim Example

`SandboxClaim` is used to create multiple instances in one shot. It does not support inline templates and must reference an existing template through `templateRef`.

```yaml
apiVersion: sandbox.kce.ksyun.com/v1alpha1
kind: SandboxClaim
metadata:
  # Child Sandbox CRs are named "<claim-name>-<index>", e.g. full-claim-0.
  name: full-claim
  namespace: sandbox-demo
spec:
  # Optional. If omitted, the Secret named sandbox-openapi-credentials in the same namespace is used.
  openapiCredentialRef:
    name: sandbox-openapi-credentials

  # Number of instances to create in one shot; must be greater than 0.
  replicas: 2

  # Reference an existing template. Use either name or id.
  templateRef:
    name: full-template
    # id: "<TEMPLATE_ID>"

  # Timeout for each instance, in seconds.
  timeoutSeconds: 3600

  # Environment variables passed to each instance. Note that these use key/value.
  env:
    - key: RUNTIME_ENV
      value: claim

  # Mount credential for each instance.
  storageCredentialRef:
    name: storage-credential

  # KS3 mount for each instance.
  ks3MountConfig:
    enabled: true
    mountPoints:
      - bucketName: ghw-test-bj-7583
        remotePath: /
        localMountPath: /mnt-new2
        readOnly: true

  # KPFS mount for each instance.
  kpfsMountConfig:
    enabled: true
    mountPoints:
      - fileSystemName: sandbox-test
        remotePath: /
        localMountPath: /mnt-new1
        readOnly: true
```

After creation, the Claim transitions through phases such as `Pending`, `Running`, `Successful`, or `Failed`, and records its child instances:

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

`SandboxClaim` is a one-shot declaration and does not support updates. Deleting the Claim does not delete the child `Sandbox` CRs that have already been created; delete those child CRs to remove the instances.
