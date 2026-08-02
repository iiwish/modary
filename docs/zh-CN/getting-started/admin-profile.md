# Admin Profile 教程

> [English](../../getting-started/admin-profile.md)

Admin Profile 面向内部运营后台。它组合普通 PostgreSQL 业务事务、开发用 Identity、
RBAC、Session/CSRF 和 React 工作界面，明确不包含 River 与受治理 Action。

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

## 配置 PostgreSQL

```bash
export DATABASE_URL='postgres://user:password@127.0.0.1:5432/app?sslmode=disable'
export MODARY_DATABASE_SCHEMA=operations_admin
export MODARY_ADMIN_USERNAME=admin
export MODARY_ADMIN_PASSWORD='development-password'
export MODARY_ALLOW_INSECURE_COOKIE=true
```

Admin 只创建应用 schema，不创建 River queue schema。`ALLOW_INSECURE_COOKIE`
仅用于本地 HTTP，生产环境保持 Secure cookie 并在应用前终止 TLS。

## 测试与运行

```bash
DATABASE_URL="$DATABASE_URL" go test ./...
go run ./cmd/operations-admin
```

打开 `http://127.0.0.1:8080`，使用配置的开发账号登录。records 示例覆盖加载、
筛选、创建、乐观更新和删除；后端 RBAC 对每条路由授权，隐藏按钮不代表授权。

## 理解后端

`internal/app/application.go` 显式选择：

- `adapters/postgresdb` 提供 `database.Store`；
- `adapters/localidentity` 提供开发身份；
- `adapters/rbac` 提供后端策略；
- `transport/sessionhttp` 提供登录、会话、退出与 CSRF；
- 应用自己的 records Module 和路由。

普通事务由 `Store.WithinTransaction` 管理。这里没有 River、Action Runtime、
SQL Audit 或 MCP。

## 理解前端

前端是生成项目自己拥有的 React 19、TypeScript、Vite 和 React Router 应用：

- `web/src/main.tsx` 只负责挂载；
- `web/src/App.tsx` 组合 Provider、会话初始化、受保护路由和 Module 路由；
- `web/src/stores/` 用小型强类型 Context 与 Hook 管理应用信息、身份和 Toast；
- `web/src/modules/index.ts` 是显式的前端组合根；
- `web/src/modules/records/` 自己拥有 API 状态、页面、对话框和路由。

新增业务功能需要明确注册路由、导航和 API，不会从后端元数据自动生成页面。
默认实现不强制引入全局状态库；只有产品出现真实的跨模块共享状态时才需要选择。

API 客户端会在变更请求中携带 CSRF token。已登录请求返回 `401` 时，本地会话会
立即清空并回到登录页；`403` 会显示独立的无权限状态。它们只改善交互，真正的
授权始终由后端完成。

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
