# Sandbox Operator

Sandbox Operator 是一个面向 Kubernetes 的沙箱资源管理组件，用于把金山云 Sandbox OpenAPI 中的沙箱模板和沙箱实例抽象为 Kubernetes CR。

当前项目提供三个 CRD：

| Kind | 简写 | 说明 |
| --- | --- | --- |
| `SandboxTemplate` | `stpl` | 沙箱模板，描述镜像、资源、网络、挂载、日志、预热池等模板配置。 |
| `Sandbox` | `sbx` | 单个沙箱实例。 |
| `SandboxClaim` | `sbxc` | 一次性批量创建多个沙箱实例的声明。 |

operator 会在以下方向做同步：

- 用户创建、更新、删除 CR 时，同步调用 Sandbox OpenAPI。
- 控制台或 OpenAPI 侧创建、修改、删除资源后，周期性同步回 Kubernetes CR。
- 删除 CR 时通过 finalizer 删除对应 OpenAPI 资源。

## 语言

- [English README](../../README.md)
- 中文文档（本页）

## 文档

- [部署说明](deployment.md)
- [使用说明](usage.md)
- [CR 示例](cr-examples.md)
- [原生 Kubernetes 部署资源说明](deploy-manifests.md)

## 目录概览

```text
api/                 CRD Go 类型定义
cmd/manager/         operator 启动入口
internal/            controller、webhook、OpenAPI client、字段映射
config/              原生 Kubernetes 部署资源和示例
charts/              Helm Chart
Makefile             常用的开发与部署任务入口
scripts/             构建、部署、卸载脚本
docs/                用户文档
```

## 开发常用命令

```bash
make build          # 编译 manager 二进制
make test           # 运行单元测试
make vet            # 运行 go vet
make lint           # 运行格式化与 vet 检查
make docker-build   # 构建容器镜像（可通过 IMG= 指定镜像名）
make deploy         # 使用原生 Manifest 部署（可通过 IMG= 指定镜像名）
make undeploy       # 卸载原生 Manifest 部署
```
