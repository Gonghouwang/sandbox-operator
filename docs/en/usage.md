# Usage Guide

This guide explains how to use Sandbox Operator through two workflows:

* **OpenAPI → Kubernetes sync**: resources are created, modified, or deleted through the console or OpenAPI, and the operator reflects them as CRs in the cluster.
* **Kubernetes → OpenAPI sync**: resources are created, modified, or deleted as Kubernetes CRs, and the operator invokes the Sandbox OpenAPI on your behalf.

## 1. Core Resources

| Kind | Short Name | Description |
| --- | --- | --- |
| `SandboxTemplate` | `stpl` | Sandbox template. |
| `Sandbox` | `sbx` | Single sandbox instance. |
| `SandboxClaim` | `sbxc` | One-shot batch declaration for creating multiple instances. |

Each business namespace must contain an OpenAPI credential Secret. The default name is `sandbox-openapi-credentials`; a different name can be specified in the CR via `spec.openapiCredentialRef.name`.

```bash
kubectl create namespace sandbox-demo

kubectl create secret generic sandbox-openapi-credentials \
  -n sandbox-demo \
  --from-literal=accessKeyId='<OPENAPI_AK>' \
  --from-literal=secretAccessKey='<OPENAPI_SK>' \
  --from-literal=accountId='<ACCOUNT_ID>' \
  --from-literal=region='cn-beijing-6'
```

The operator synchronizes only namespaces where it can read OpenAPI credentials. Templates, sandboxes, and credential Secrets must live in the same business namespace.

## 2. OpenAPI → Kubernetes Sync

### 2.1 Sync entry point

The operator's internal poller periodically scans namespaces that contain an OpenAPI credential Secret and calls the OpenAPI to list templates and instances. Synchronization results are written into CRs in the corresponding namespace:

```bash
kubectl get stpl -n sandbox-demo
kubectl get sbx -n sandbox-demo
```

The OpenAPI is the source of truth. After a resource is modified through the console or OpenAPI, the next sync round overwrites the CR's `spec` and `status`.

### 2.2 Template sync

After a template is created through the console or OpenAPI, the operator creates a `SandboxTemplate` CR. Naming rules are:

* If the OpenAPI template name is a valid Kubernetes name, the CR uses the template name.
* If the template name is invalid, the CR uses the full template ID.
* The platform template ID is stored in the annotation `sandbox.kce.ksyun.com/template-id`, not in `status`.

Inspect templates:

```bash
kubectl get stpl -n sandbox-demo
kubectl get stpl -n sandbox-demo <template-name> -o yaml
```

The following field mapping applies:

| OpenAPI / Console field | CR field |
| --- | --- |
| template ID | `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` |
| description | `spec.description` |
| type | `spec.type` |
| access | `spec.access` |
| image | `spec.template.spec.image` |
| CPU, memory, instance type, system disk, data disks | `spec.template.spec.kecConfig` |
| ports | `spec.template.spec.ports` |
| start command | `spec.template.spec.startCommand` |
| environment variables | `spec.template.spec.env` |
| KS3 mount | `spec.template.spec.ks3MountConfig` |
| KPFS mount | `spec.template.spec.kpfsMountConfig` |
| network | `spec.template.spec.networkConfig` |
| skill configuration | `spec.template.spec.skillConfig` |
| logging configuration | `spec.template.spec.observability` |
| preheat pool target size | `spec.template.spec.pool.targetSize` |
| OpenAPI update time | `status.externalUpdatedAt` |
| deletable flag | `status.canDelete` |
| preheat pool status | `status.preheat` |

When `spec.template.spec.networkConfig` is omitted or set to `{}`, the operator calls OpenAPI with public access only and `SharedInternetAccessEnable=true`.

CRs synced from OpenAPI do not automatically create or write back Kubernetes Secrets. Image registry, KS3, KPFS, Klog, and other credentials are not reverse-generated from OpenAPI. If you later need to modify image or mount-related fields in the cluster, create the corresponding Secrets in the same namespace and add `registryCredentialRef` or `storageCredentialRef` to the CR.

### 2.3 Sandbox instance sync

After a sandbox instance is created through the console or OpenAPI, the operator creates a `Sandbox` CR. Naming rules are:

* If the instance has a valid name, the CR uses the instance name.
* If the instance name is empty or invalid, the CR uses the full instance ID.
* The platform instance ID is stored in the annotation `sandbox.kce.ksyun.com/sandbox-id`.
* The platform template ID is stored in the annotation `sandbox.kce.ksyun.com/template-id`.

Inspect instances:

```bash
kubectl get sbx -n sandbox-demo
kubectl get sbx -n sandbox-demo <sandbox-name> -o yaml
```

The following field mapping applies:

| OpenAPI / Console field | CR field |
| --- | --- |
| instance ID | `metadata.annotations["sandbox.kce.ksyun.com/sandbox-id"]` |
| template ID | `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` and `spec.templateRef.id` |
| timeout | `spec.timeoutSeconds` and `status.timeoutSeconds` |
| instance phase | `status.phase` |
| access endpoint | `status.endpoint`, `status.urls`, `status.accessUrl` |
| token | `status.token` |
| image | `status.imageUrl` |
| start command | `status.command` |
| instance env | `status.env` |
| instance KS3 mount | `status.ks3MountConfig` |
| instance KPFS mount | `status.kpfsMountConfig` |
| end time | `status.endTime` |

Instance-level environment variables and mount configurations override template-level settings. After the timeout is modified through the console, the next sync updates both `spec.timeoutSeconds` and `status.timeoutSeconds`.

### 2.4 Deletion or reclamation from OpenAPI

When a resource no longer exists on the OpenAPI side, the operator deletes the corresponding CR:

* Deleted templates cause the corresponding `SandboxTemplate` to be removed from the cluster.
* Deleted or expired-reclaimed instances cause the corresponding `Sandbox` to be removed from the cluster.

Therefore, do not treat CRs synced from OpenAPI as long-lived local copies; they track the authoritative OpenAPI state.

## 3. Kubernetes → OpenAPI Sync

### 3.1 Create a template

Prepare a `SandboxTemplate`; full examples are in [CR examples](cr-examples.md#2-full-sandboxtemplate-example).

```bash
kubectl apply -f template.yaml
kubectl get stpl -n sandbox-demo
kubectl get stpl -n sandbox-demo full-template -o yaml
```

When the CR is created, the mutating webhook first calls OpenAPI to create the real template and then injects the platform template ID into the CR's annotations:

```bash
kubectl get stpl -n sandbox-demo full-template \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/template-id}{"\n"}'
```

Before creating a template with a private image, KS3, or KPFS mounts, create the runtime credential Secrets first. See [Credential Secret examples](cr-examples.md#1-credential-secret-examples).

### 3.2 Update a template

When you modify `SandboxTemplate.spec`, the validating webhook computes the diff and calls the OpenAPI template update interface. Currently the following fields support diff-based updates:

| CR field | Description |
| --- | --- |
| `spec.description` | Template description. |
| `spec.access` | Private / Public. Public templates cannot configure a pool. |
| `spec.type` | AIO / Browser / Code / Custom; case input is accepted compatibly. |
| `spec.template.spec.image` | Image source, image address, and registry credential. |
| `spec.template.spec.ports` | Exposed ports. |
| `spec.template.spec.startCommand` | Start command. |
| `spec.template.spec.kecConfig.cpu` | CPU. |
| `spec.template.spec.kecConfig.memory` | Memory. |
| `spec.template.spec.kecConfig.instanceSpecs` | Multiple instance-type configuration; each item contains `instanceType`, `systemDisk`, and optional `dataDisks`. |
| `spec.template.spec.kecConfig.instanceSpecs[].instanceType` | KEC instance type. |
| `spec.template.spec.kecConfig.instanceSpecs[].systemDisk` | System disk type and size. |
| `spec.template.spec.kecConfig.instanceSpecs[].dataDisks` | Data disks. The field remains an array for future multi-disk compatibility; currently each instance type supports at most one data disk. |
| `spec.template.spec.env` | Template environment variables, using `name`/`value`. |
| `spec.template.spec.networkConfig` | Network configuration. |
| `spec.template.spec.skillConfig` | Skill configuration. |
| `spec.template.spec.ks3MountConfig` | KS3 mount. |
| `spec.template.spec.kpfsMountConfig` | KPFS mount. |
| `spec.template.spec.observability` | Logging configuration. |
| `spec.template.spec.pool` | Private template preheat pool target size. |

Example:

```bash
kubectl edit stpl -n sandbox-demo full-template
```

If you modify KS3/KPFS mounts:

* To remove all KS3 mounts, delete `ks3MountConfig` or set `enabled: false`.
* To remove all KPFS mounts, delete `kpfsMountConfig` or set `enabled: false`.
* As long as the updated KS3 or KPFS mount remains enabled, `spec.template.spec.storageCredentialRef.name` must be configured and the referenced Secret must contain `accessKey` and `secretAccessKey`.

If you modify instance type, system disk, or data disks, configure them through `spec.template.spec.kecConfig.instanceSpecs`. Each item must set `instanceType`, `systemDisk.type`, and `systemDisk.size`. `dataDisks` currently accepts at most one data disk and does not require `snapshotID`. Direct `instanceType`, `systemDisk`, and `dataDisks` fields under `kecConfig` are no longer supported.

### 3.3 Delete a template

```bash
kubectl delete stpl -n sandbox-demo full-template
```

Template deletion first checks whether OpenAPI allows deletion:

* If `status.canDelete=false`, or the OpenAPI returns `CanDelete=false`, the delete request is rejected by the webhook with an error message.
* If deletion is allowed, the CR enters the deletion flow and the reconciler calls OpenAPI to delete the real template through a finalizer before removing the finalizer.

When instances still reference the template, you generally need to delete the instances first and then delete the template.

### 3.4 Create a sandbox instance

Prepare a `Sandbox`; full examples are in [CR examples](cr-examples.md#4-full-sandbox-example).

```bash
kubectl apply -f sandbox.yaml
kubectl get sbx -n sandbox-demo
kubectl get sbx -n sandbox-demo full-sandbox -o yaml
```

When the CR is created, the mutating webhook calls OpenAPI to start the instance and injects these annotations:

* `sandbox.kce.ksyun.com/sandbox-id`
* `sandbox.kce.ksyun.com/template-id`
* `sandbox.kce.ksyun.com/endpoint`
* `sandbox.kce.ksyun.com/token`

The reconciler then queries OpenAPI by instance ID and writes back the `status`.

`Sandbox` supports two creation methods:

* `spec.templateRef.name` or `spec.templateRef.id`: creates an instance from an existing template.
* `spec.template`: inline template. The operator first creates the template and then creates an instance from it.

### 3.5 Update a sandbox instance

`Sandbox` currently supports updating only `spec.timeoutSeconds`:

```bash
kubectl patch sbx -n sandbox-demo full-sandbox --type=merge -p '{
  "spec": {
    "timeoutSeconds": 3600
  }
}'
```

Updating `spec.name`, `spec.templateRef`, `spec.template`, `spec.env`, `spec.ks3MountConfig`, or `spec.kpfsMountConfig` is not supported. The sandbox name is based on `metadata.name`; `spec.name` should not be configured.

### 3.6 Delete a sandbox instance

```bash
kubectl delete sbx -n sandbox-demo full-sandbox
```

After the CR is deleted, the reconciler calls OpenAPI to delete the real instance through a finalizer and then removes the finalizer.

### 3.7 Create a SandboxClaim

`SandboxClaim` is a one-shot batch creation declaration. Full examples are in [CR examples](cr-examples.md#6-full-sandboxclaim-example).

```bash
kubectl apply -f claim.yaml
kubectl get sbxc -n sandbox-demo full-claim -o yaml
kubectl get sbx -n sandbox-demo -l sandbox.kce.ksyun.com/claim=full-claim
```

When the Claim is created, the webhook calls OpenAPI to start the number of instances specified by `spec.replicas`. The reconciler materializes these instances as child `Sandbox` CRs named `<claim-name>-<index>`.

After the Claim reaches `Successful` or `Failed`, it stops creating further instances and does not replace children that are reclaimed by the platform. `SandboxClaim` does not support updates; to change the replica count, timeout, environment variables, or mounts, create a new Claim.

Deleting a `SandboxClaim` deletes only the declaration itself, not the `Sandbox` CRs and underlying instances already created:

```bash
kubectl delete sbxc -n sandbox-demo full-claim
```

If you need to delete instances created by the Claim, delete the corresponding child `Sandbox` CRs.

## 4. Common Status Fields

### 4.1 SandboxTemplate

| Field | Description |
| --- | --- |
| `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` | Platform template ID. |
| `status.phase` | Template phase. |
| `status.canDelete` | Whether the template can be deleted. |
| `status.externalUpdatedAt` | Update time on the OpenAPI side. |
| `status.createdAt` / `status.updatedAt` | Create and update times on the OpenAPI side. |
| `status.quota` | Template quota information. |
| `status.preheat` | Private template preheat pool status; not present for Public templates. |
| `status.conditions` | Sync result and exception information. |

### 4.2 Sandbox

| Field | Description |
| --- | --- |
| `metadata.annotations["sandbox.kce.ksyun.com/sandbox-id"]` | Platform instance ID. |
| `metadata.annotations["sandbox.kce.ksyun.com/template-id"]` | Platform template ID. |
| `status.phase` | Instance phase. |
| `status.timeoutSeconds` | Instance timeout. |
| `status.endpoint` | Instance endpoint. |
| `status.urls` / `status.accessUrl` | Access URLs. |
| `status.token` | Access token. |
| `status.imageUrl` | Instance image. |
| `status.command` | Start command. |
| `status.env` | Instance environment variables. |
| `status.ks3MountConfig` / `status.kpfsMountConfig` | Instance mount configuration. |
| `status.endTime` | Instance end time. |
| `status.conditions` | Sync result and exception information. |

### 4.3 SandboxClaim

| Field | Description |
| --- | --- |
| `status.phase` | Claim phase. |
| `status.desired` | Desired number of instances. |
| `status.created` | Number of child Sandbox CRs created. |
| `status.ready` | Number of Running child Sandbox CRs. |
| `status.failed` | Number of Failed/Unhealthy child Sandbox CRs. |
| `status.sandboxes` | Child Sandbox names and phases. |
| `status.conditions` | Processing result and exception information. |

## 5. Troubleshooting Commands

View operator logs:

```bash
kubectl logs -n sandbox-operator-system deploy/sandbox-operator
```

Check whether OpenAPI credentials exist:

```bash
kubectl get secret -n sandbox-demo sandbox-openapi-credentials -o yaml
```

Find why a template deletion was rejected:

```bash
kubectl get stpl -n sandbox-demo full-template -o yaml
```

Pay attention to:

* `status.canDelete`
* `status.conditions`
* `metadata.deletionTimestamp`
* `metadata.finalizers`

Inspect an instance and its platform ID:

```bash
kubectl get sbx -n sandbox-demo full-sandbox \
  -o jsonpath='{.metadata.annotations.sandbox\.kce\.ksyun\.com/sandbox-id}{"\n"}'
```

## 6. Frequently Asked Questions

### OpenAPI resources are not synced into the cluster

Confirm that the business namespace contains the OpenAPI credential Secret and that the Secret includes `accessKeyId`, `secretAccessKey`, and `region`. The operator scans only namespaces where it can read credentials.

### Creating or updating mount configuration fails

KS3/KPFS mounts require `storageCredentialRef` to point to a Secret in the same namespace. The Secret must contain `accessKey` and `secretAccessKey`.

### Updating Sandbox environment variables, mounts, or template reference is rejected

`Sandbox` currently supports updating only `spec.timeoutSeconds`. Other fields take effect only when the instance is created.

### Public template pool configuration is rejected

Public templates do not support preheat pools. Remove `spec.template.spec.pool` and resubmit.

### Deleting a template fails

Instances still reference the template. Delete the dependent `Sandbox` CRs first, then delete the template.
