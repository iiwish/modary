# Admin Profile 教程

> [English](../../getting-started/admin-profile.md)

Admin Profile 面向内部运营后台。它组合普通 PostgreSQL 业务事务、开发用 Identity、
RBAC、Session/CSRF 和 React 工作界面。River 任务检查和 SQL 审计检查属于显式的
生成时选择；受治理 Action 仍由独立 Profile 承担。

## 创建

```bash
export MODARY_STARTER_REPLACE="$(pwd)"
go run ./cmd/modary new ../operations-admin \
  --profile admin \
  --module example.com/acme/operations-admin \
  --name "Operations Admin"
cd ../operations-admin
go mod tidy
```

默认生成最小 Admin。需要运维读取界面时显式选择：

```bash
go run ./cmd/modary new ../operations-admin \
  --profile admin \
  --with tasks \
  --with audit \
  --module example.com/acme/operations-admin
```

`--with tasks` 会选择 `components/governedpostgres`、独立 River schema、受限的
`task.Inspector`、`/api/tasks` 和 Tasks React 模块。`--with audit` 会选择 SQL
Audit、按 scope 约束的 `audit.Reader`、`/api/audit` 和 Audit log React 模块。
未选择的组件不会进入 Go 依赖图、生成源码、路由、导航或生产 bundle。

## 配置 PostgreSQL

```bash
export DATABASE_URL='postgres://user:password@127.0.0.1:5432/app?sslmode=disable'
export MODARY_DATABASE_SCHEMA=modary_app_operations_admin
export MODARY_QUEUE_SCHEMA=modary_queue_operations_admin # 仅 --with tasks 需要
export MODARY_ADMIN_USERNAME=admin
export MODARY_ADMIN_PASSWORD='development-password'
export MODARY_ALLOW_INSECURE_COOKIE=true
```

默认 Admin 只创建应用 schema。选择 tasks 后会在同一数据库创建独立 River
schema，以保留业务写入与任务入队的原子性。Starter 根据项目 ID 派生带角色前缀的
默认名称，使 application、queue 与 test 命名空间彼此分离，并避开 PostgreSQL
保留 schema；名称超过 PostgreSQL 或 River 上限时会保留可读前缀并加入确定性哈希
片段。`ALLOW_INSECURE_COOKIE` 仅用于本地 HTTP，生产环境保持 Secure cookie 并在
应用前终止 TLS。

## 测试与运行

```bash
DATABASE_URL="$DATABASE_URL" go test ./...
go run ./cmd/operations-admin migrate
go run ./cmd/operations-admin serve
```

`migrate` 只应用已选择的前向迁移并退出；`serve` 不会隐式修改 schema。只有明确的
本地单进程流程才应设置 `MODARY_MIGRATE_ON_START=true`。

打开 `http://127.0.0.1:8080`，使用配置的开发账号登录。records 示例覆盖加载、
筛选、创建、乐观更新和删除；后端 RBAC 对每条路由授权，隐藏按钮不代表授权。

## 理解后端

`internal/app/application.go` 显式选择：

- `components/postgres` 提供 `database.Store`；
- `components/postgres/identitystore` 提供开发身份；
- `components/postgres/rbac` 提供后端策略；
- `transport/sessionhttp` 提供登录、会话、退出与 CSRF；
- 应用自己的 records Module 和路由。

普通事务由 `Store.WithinTransaction` 管理。这里没有 River、Action Runtime、
SQL Audit 或 MCP。

## 理解前端

前端是生成项目自己拥有的 React 19、TypeScript、Vite 和 React Router 应用：

- `web/src/main.tsx` 只负责挂载；
- `web/src/App.tsx` 组合 Provider、会话初始化、受保护路由和 Module 路由；
- `web/src/stores/` 用小型强类型 Context 与 Hook 管理应用信息、身份和 Toast；
- `web/src/modules/active.ts` 是生成时的显式组件选择；
- `web/src/modules/index.ts` 使用后端 descriptor 与当前权限解析选中的源码模块；
- `web/src/modules/records/` 自己拥有 API 状态、页面、对话框和路由。

新增业务功能需要明确注册路由、导航和 API，不会从后端元数据自动生成页面。
登录后的 `/api/admin/context` 只提供 label、path、权限清单和当前用户 grants，
绝不提供可执行 UI；未知或未授权 descriptor 会关闭显示，后端仍逐请求鉴权。
默认实现不强制引入全局状态库；只有产品出现真实的跨模块共享状态时才需要选择。

生成的壳层将 `/api` 与 `/readyz` 排除在 SPA fallback 之外。未知后端路径会在
所有 `Accept` 请求头下保持 HTTP 404，而不会被 `index.html` 掩盖。

API 客户端会在变更请求中携带 CSRF token。已登录请求返回 `401` 时，本地会话会
立即清空并回到登录页；`403` 会显示独立的无权限状态。它们只改善交互，真正的
授权始终由后端完成。

生成的管理界面以简体中文（`zh-CN`）为主要语言，导航、命令、状态、校验反馈和
无障碍标签均使用中文。技术标识符、任务类型、队列名、请求 ID 和审计源数据保持原值。

```bash
cd web
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm assets:check
pnpm audit:prod
```

`pnpm build` 更新 `internal/web/dist`；`assets:check` 在临时目录重建并要求生产
资源逐字节一致。入口 HTML 使用 `Cache-Control: no-cache`；JavaScript 与 CSS
使用内容哈希文件名、ETag 和 `Cache-Control: public, max-age=31536000,
immutable`。资源内容变化时 URL 同步变化，未变化资源可以继续长期缓存。
`audit:prod` 使用公共 npm advisory 服务阻止已知的高危或严重生产依赖漏洞。

## 生产替换项

替换开发 Identity，启用 TLS 和安全 Cookie，配置真实角色与 scope，增加限流、
备份、恢复和观测，并把 records 示例替换成产品 Module。前端源码属于应用，可以
继续演进或完全替换。
