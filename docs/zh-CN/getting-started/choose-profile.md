# 如何选择 Profile

> [English](../../getting-started/choose-profile.md)

Profile 是创建项目时选择的一组组件，不是运行时开关。没有选择的组件不会出现在
生成源码或依赖图中。

| Profile | 适用场景 | 默认包含 | 明确不包含 |
|---|---|---|---|
| API | 小型 API、网关、暂未确定存储方案的服务 | Core、显式 HTTP 路由、健康检查 | 数据库、登录、RBAC、River、Action、前端 |
| Admin | 内部运营后台、普通业务 CRUD | PostgreSQL Store、本地 Identity、RBAC、Session/CSRF、React UI | River、受治理 Action、SQL Audit、MCP |
| Governed | 高影响变更、自动化控制面、严格审计和耐久任务 | PostgreSQL/River、Identity、RBAC、Audit、Action、CLI/HTTP/MCP、worker | Admin UI |

## 选择原则

从满足当前真实需求的最小 Profile 开始。普通列表、表单和草稿保存不需要为了
“统一”而进入 Preview/Execute。只有操作的影响、自动化、审计或重试需求值得额外
复杂度时，才选择 Governed。

一个产品可以从 Admin 开始，后续只为发布、部署、批量操作或 AI 自动执行等少数
高影响路径组合 Governed 组件。不要把 Profile 当成互斥的永久产品类型。

## 数据库选择

Core 和 API Profile 不需要数据库。Admin 使用普通 `database.Store`，官方 F0
实现是 PostgreSQL。Governed 使用 PostgreSQL 和 River；River 只需要同一数据库
中的独立 schema，不要求专用数据库。

F0 暂无官方 MySQL 或 SQLite Adapter。应用可以实现自己的 Store，但 Modary 不对
其迁移、事务和生产质量作出默认承诺。
