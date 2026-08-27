# Modary

**构建从小处起步、始终保持显式的 Go 后端。**

一个面向业务系统和中后台的组件化 Go 框架。从无数据库 Core 开始，只添加产品真正
需要的基础设施。

[![CI](https://github.com/iiwish/modary/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/iiwish/modary/actions/workflows/ci.yml)
[![Release v0.3.0-alpha.2](https://img.shields.io/badge/release-v0.3.0--alpha.2-2f6f4e)](https://github.com/iiwish/modary/tree/v0.3.0-alpha.2)
[![Go reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/iiwish/modary)
[![Apache 2.0 license](https://img.shields.io/badge/license-Apache--2.0-5a6472)](LICENSE)

[为什么选择 Modary](#为什么选择-modary) · [快速开始](#快速开始) ·
[Profile](#选择-profile) · [架构](#架构) ·
[文档](docs/zh-CN/index.md) · [参与贡献](CONTRIBUTING.md) ·
[English](README.md)

---

Modary 为 Go 团队提供精简的模块化单体 Core，以及一条从普通 CRUD 走向高影响操作
的清晰路径。对于需要 Preview、授权、幂等、审计和耐久任务的操作，可以按需采用更
严格的执行模型。框架不扫描 package，不使用全局 service locator，也不接管产品的
领域模型。应用显式列出自己的 Module，并拥有生成后的全部代码。

> [!IMPORTANT]
> Modary 是 v1 之前的 Alpha 软件。当前组件框架版本为 `v0.3.0-alpha.2`，要求
> Go 1.26.7 或更高版本。请精确固定版本，并在升级前阅读
> [CHANGELOG](CHANGELOG.md)。

## 为什么选择 Modary

当完整功能集恰好匹配产品时，大型后台 Starter 很高效。否则，其中的数据库模型、
权限体系、任务系统、菜单和 UI 约定很快会变成意外的架构负担。Modary 让这些选择
保持可见，并且可以真正移除。

| 原则 | 在实际项目中的含义 |
|---|---|
| **默认精简** | Core 不依赖数据库、任务队列、Identity、Action Runtime、MCP server 或前端。 |
| **显式组合** | 一个 `appkit.Definition` 声明进程中使用的全部 Module 和 Capability。 |
| **应用拥有产品代码** | 领域代码、schema、路由、策略、品牌、部署和发布都留在应用仓库中。 |
| **按风险选择严格度** | 普通 CRUD 使用常规事务；只有需要 Preview、审计、幂等或耐久任务时才选择受治理 Action。 |
| **可证明的缺省** | 未选择的 Adapter 不会贡献 migration、路由、配置、goroutine、服务、源码 Module 或生产 Bundle。 |

Modary 适合希望获得框架支持、同时保留普通 Go 代码库所有权的团队，用于构建内部
工具、运营控制台、业务 API 和受治理工作流。

## 快速开始

使用已发布的 Starter 创建最精简、无数据库的 API Profile：

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.2 \
  new sample-api \
  --profile api \
  --module example.com/acme/sample-api

cd sample-api
go mod tidy
go test ./...
go run ./cmd/sample-api
```

在另一个终端中执行：

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/ping
# {"message":"pong"}
```

生成的项目由普通 Go 源码组成。它的组合根足够简洁，可以一眼检查：

```go
func Definition() appkit.Definition {
	return appkit.Definition{
		Metadata: appkit.Metadata{
			ID:      "sample-api",
			Name:    "sample-api",
			Version: "0.1.0",
		},
		Modules: []module.Registration{
			ping.Registration(),
		},
	}
}
```

接下来可以完成[五分钟快速上手](docs/zh-CN/getting-started/quickstart.md)，或阅读
[添加第一个业务 Module](docs/how-to/add-module.md)。

## 选择 Profile

Profile 是只在创建项目时使用的源码起点，不是运行时模式。Starter 只会向新项目
复制源码，绝不会修补已有项目。从创建完成开始，所有文件和 Module 列表都归应用
所有。

| Profile | 适用场景 | 默认选择 | 明确不包含 |
|---|---|---|---|
| [`api`](docs/zh-CN/getting-started/first-application.md) | 服务和轻量 API | Core、进程探针、示例路由 | 数据库、Identity、UI、任务、审计、Action、MCP、OTel |
| [`admin`](docs/zh-CN/getting-started/admin-profile.md) | 内部工具和业务后台 | PostgreSQL Store、本地 Identity、RBAC、Session、内嵌 React Admin | 受治理 Action 和 MCP；任务、审计、OIDC、OTel 均为显式可选项 |
| [`governed`](docs/zh-CN/getting-started/governed-profile.md) | 高影响命令和耐久工作流 | PostgreSQL、River、Identity、RBAC、SQL Audit、CLI/HTTP/MCP Action、worker | Admin UI 和普通 records 功能切片 |

创建 Admin 项目时可以显式选择 `tasks`、`audit`、`oidc` 和 `otel`。完整决策路径与
组件边界参见[选择 Profile](docs/zh-CN/getting-started/choose-profile.md)。

## 架构

```text
consumer command / HTTP server / worker
                 |
                 v
        appkit.Definition
          显式注册 Module
                 |
                 v
      module.Host + typed Capability
        /              |              \
       /               |               \
   无数据库 API     Admin CRUD      受治理 Action
                     Store           Runtime
                                       |
                            PostgreSQL + River
```

Core 负责 Module 校验、依赖排序、typed Capability、生命周期和不透明的应用组装。
可选组件提供持久化、Identity、授权、Session、任务、审计、可观测性和 Transport。
业务行为始终由 consumer Module 拥有。

### 受治理 Action

对于提交前需要确认影响范围的操作，可选 Action Runtime 提供一条严格、统一的执行
路径：

```text
authorize intent -> Preview -> bind plan -> authorize impact
-> transaction -> reauthorize -> idempotency
-> mutation + durable task + audit
```

这条路径是可选的。普通 Admin CRUD 不需要 Preview 或 River，而是使用受限的
`database.Store`；只有被明确选择的操作才使用受治理 Action。

### Admin UI

可选 Admin Profile 会生成由应用拥有的 React 19 和 TypeScript 工作界面，包含
Session 恢复、CSRF 防护、权限感知导航、响应式 scoped CRUD 和无障碍 Dialog。
生产 Bundle 会内嵌到 Go 二进制文件中，因此部署后的应用不需要 Node.js。它是参考
工作界面，不是低代码 schema 或动态菜单引擎。

## Package 地图

| 分层 | 主要 package |
|---|---|
| Core 与组合 | `module`、`appkit`、`appcmd`、`httpkit`、`processkit` |
| 窄接口契约 | `database`、`identity`、`authz`、`scope`、`task`、`action`、`audit`、`observe` |
| 标准组件 | `components/postgres`、`components/governedpostgres`、`components/oidc`、`components/otel` |
| Transport | `transport/httpapi`、`transport/sessionhttp` |
| 项目创建 | `starter`、`cmd/modary` |
| 可选项目工具 | `projecttool` |

普通 PostgreSQL 组件不依赖 River 或受治理 Action 持久化。Governed PostgreSQL
组件会有意添加这些能力。导入方式参见[公共 package 地图](docs/reference/packages.md)。

## 文档

| 目标 | 从这里开始 |
|---|---|
| 理解整体模型 | [选择 Profile](docs/zh-CN/getting-started/choose-profile.md)、[Module 与 Capability](docs/concepts/modules-and-capabilities.md) |
| 构建应用 | [快速上手](docs/zh-CN/getting-started/quickstart.md)、[Admin 教程](docs/zh-CN/getting-started/admin-profile.md)、[Governed 教程](docs/zh-CN/getting-started/governed-profile.md) |
| 生产运行 | [部署](docs/zh-CN/operations/deployment.md)、[可观测性](docs/zh-CN/operations/observability.md)、[安全边界](docs/operations/security.md) |
| 核对精确支持范围 | [支持矩阵](docs/reference/support-matrix.md)、[已知限制](docs/f0-known-limitations.md)、[F0 契约](docs/framework-f0.md) |
| 阅读英文 README | [English](README.md) |

中文教程的完整入口位于 [`docs/zh-CN/index.md`](docs/zh-CN/index.md)；包含全部教程、
运维指南、ADR 和发布说明的文档地图位于 [`docs/index.md`](docs/index.md)。

## 生产边界

- PostgreSQL 是 F0 唯一的官方耐久数据库。项目不提供 MySQL 或嵌入式数据库
  Adapter。
- 本地 Identity 面向开发和受控的内部部署，并不是完整的公网 IAM 系统。
- 耐久任务采用 at least once 交付。Handler 必须可幂等执行并正确响应取消。
- Modary 不是操作系统 sandbox、分布式事务协调器、数据库 Operator 或部署安全
  边界。

投入生产前，请阅读[支持矩阵](docs/reference/support-matrix.md)、
[安全策略](SECURITY.md)和[已知限制](docs/f0-known-limitations.md)。

## 参与贡献

欢迎能够维护框架与 consumer 边界、并让可选基础设施保持真正可选的贡献。请先阅读
[`CONTRIBUTING.md`](CONTRIBUTING.md)，然后运行常规验收套件：

```bash
make bootstrap
make acceptance
make race
```

仅修改文档时可以运行 `make docs-check`。安全问题必须通过
[私密报告流程](SECURITY.md)提交，请勿创建公开 Issue。

## License

Modary 基于 [Apache License 2.0](LICENSE) 发布。
