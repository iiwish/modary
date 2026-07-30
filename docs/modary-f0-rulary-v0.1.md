# Modary F0 Delivery Contract — Rulary Vertical Slice

> 项目：**Modary（工作名）**
> 里程碑：**F0**
> 唯一验收对象：**Rulary 地址标签纵向切片**
> 文档类型：Milestone Delivery Contract
> 版本：v0.1
> 日期：2026-07-30
> 状态：Accepted / AC-001 至 AC-015 验收通过
> 上位文档：`Modary SSOT v0.1`
> 原则：**验证平台内核，不顺手开发完整 Rulary。**

---

## 0. 文档目的

本文件是 Modary F0 的唯一交付合同，回答：

- F0 具体做什么。
- F0 明确不做什么。
- Rulary 如何作为真实验收对象。
- 哪些技术边界必须在首版证明。
- 如何判定 F0 成功或失败。
- 范围失控时先砍什么。

本文件不修改 Rulary 的长期产品定位。Rulary 仍然是 Chat-first 数据标签 RuleOps 工作台；F0 只抽取一个足以验证 Modary 的纵向切片。

---

## 1. F0 目标

F0 只验证一个核心假设：

> **一个真实垂直产品能否通过可静态组合的 Module 贡献类型化 Action，并让 React UI、HTTP API、CLI 与 Agent 使用同一个受控 Runtime 完成权限、预览、执行和审计。**

F0 成功后，应该能够展示：

```text
安装 Rulary Module
→ 构建单一应用
→ 在 UI 中创建并预览地址规则
→ 通过同一个 Action 从 API / Agent 调用
→ 权限不能被绕过
→ 执行前看到影响
→ 执行后有统一审计
→ 最终仍是一个 Go 二进制 + 一个 SQLite 文件
```

F0 不以功能数量、通用 CRUD 覆盖率或插件市场规模作为成功标准。

---

## 2. 为什么选择 Rulary

Rulary 不是玩具 Todo。它天然包含平台需要验证的关键压力点：

- 规则草稿与不可变版本。
- 真实数据 Preview。
- 发布权限。
- 批量执行与影响行数。
- 执行结果与证据。
- 审计和可追溯。
- 未来 Agent 操作入口。
- 轻量单机部署要求。

Rulary 的完整长期链路是：

```text
描述意图
→ 生成 / 修改 RuleSpec
→ 真实数据预览
→ 发布版本
→ 定时或手动运行
→ 结果写回
→ 审核与追溯
```

Modary F0 只实现其中足以验证平台的部分：

```text
创建确定性 RuleSpec
→ Validate
→ Preview
→ Publish
→ Execute
→ Audit
```

Chat、AI 生成、调度、跨数据库与完整审核中心不进入本里程碑。

---

## 3. F0 验收场景

### 3.1 输入数据

本地 SQLite 中包含测试表：

```sql
create table company_license (
  company_id text primary key,
  company_name text,
  license_address text,
  updated_at text
);
```

核心样例：

```text
平顶山市卫东区建设路东段南4号院（移动公司办公楼西200米）；
（经营地址备案：平顶山市黄河路与高新大道交叉口尼龙织造产业园内办公楼50号）
```

### 3.2 期望输出

```json
{
  "registered_address": "平顶山市卫东区建设路东段南4号院",
  "business_address": "平顶山市黄河路与高新大道交叉口尼龙织造产业园内办公楼50号",
  "address_note": "移动公司办公楼西200米",
  "has_business_address_filing": true,
  "address_quality_tag": "含经营地址备案"
}
```

同时保存：

```text
rule_version
run_id
evidence
processed_at
```

### 3.3 业务验收流程

1. `rule_author` 登录 Modary Console。
2. 创建一个 Rulary RuleSet 草稿。
3. 使用结构化表单或 JSON 专家视图录入最小 RuleSpec。
4. 调用 `rulary.ruleset.validate`。
5. 调用 `rulary.ruleset.preview`，查看原始值、候选结果、证据与影响行数。
6. `rule_publisher` 发布不可变版本。
7. `run_operator` 对测试数据执行已发布版本。
8. 结果写入 `company_address_labels`。
9. `auditor` 查看完整 Action 审计链。
10. 一个受限 Agent 使用同一个 `rulary.run.execute` 先 Preview，再携带有效 `plan_hash` 执行；超出 `max_rows` 或权限范围时必须被拒绝。

---

## 4. F0 产品切片

### 4.1 平台内核

F0 必须实现：

```text
Module manifest loader
Module dependency verifier
Generated static registry
Action registry
Action Runtime
Authorizer interface
Audit hook
Module migration coordinator
Generic HTTP Action Gateway
Minimal diagnostics
```

### 4.2 官方基础模块

F0 组合以下模块：

| Module | 类型 | 职责 |
|---|---|---|
| `database-sqlite` | Adapter | 系统库、事务、Module migrations |
| `identity-local` | Adapter | 本地用户、密码与数据库 Session |
| `authz-basic` | Adapter | RBAC + workspace scope + Agent grant |
| `audit` | Feature | Append-only Action audit 与查询页 |
| `console-react` | Feature | 登录、Shell、路由与 Action 交互组件 |
| `agent-mcp` | Adapter | 将 allowlist Action 暴露为 MCP Tool |
| `rulary-core` | Feature | RuleSet、Version、Preview、Run 与地址规则 |

F0 不要求每个基础能力都独立仓库；先在 monorepo 中保持模块边界。

### 4.3 Rulary 领域范围

F0 只实现：

- Workspace。
- RuleSet。
- RuleVersion。
- RuleRun。
- Address Label Result。
- Rulary 自有的确定性地址解析 Operator。
- RuleSpec 最小保存、校验和版本化。
- Preview。
- 手动 Execute。
- 结果追溯。

---

## 5. 明确非目标

F0 **不做**：

- 完整 Rulary Chat。
- AI 生成或修改 RuleSpec。
- AI 模型 Provider。
- 通用规则可视化画布。
- 完整决策表。
- 调度、goqite、Worker 分片与增量水位。
- PostgreSQL / MySQL 系统库。
- 外部业务数据库连接中心。
- 完整通用 Resource Renderer。
- 运行时热安装或热卸载。
- 远程 Module Registry。
- 复杂语义化依赖求解。
- CEL Policy Studio。
- 完整 ABAC / ReBAC。
- OpenFGA、SpiceDB、OPA 或 Casbin Adapter。
- Casdoor 集成。
- 多级 Agent 委托。
- 工作流、多人审批流与补偿框架。
- S3、邮件、短信、支付和计费。
- 微服务拆分。
- Kubernetes。
- Fluxale 迁移。

这些能力可以在 F1 之后评估，但不得以“以后肯定需要”为理由进入 F0。

---

## 6. F0 Module Contract

### 6.1 Manifest

F0 使用最小 YAML：

```yaml
schemaVersion: modary.module/v1alpha1

id: rulary-core
version: 0.1.0
type: feature

requires:
  - database
  - identity
  - authorization
  - audit

provides:
  - rulary

actions:
  - rulary.ruleset.create
  - rulary.ruleset.update_draft
  - rulary.ruleset.validate
  - rulary.ruleset.preview
  - rulary.ruleset.publish
  - rulary.run.execute

migrations:
  sqlite: migrations/sqlite

ui:
  routes:
    - id: rulary.rules
      path: /rulary/rules
      entry: ./ui/routes.tsx
```

### 6.2 F0 验证规则

`modary verify` 必须检查：

1. Module ID 唯一。
2. 所有 `requires` 被唯一能力提供者满足。
3. 依赖图无环。
4. Action ID 唯一。
5. UI Route ID 与 Path 不冲突。
6. Migration ID 不重复。
7. Feature Module 不导入其他 Module 的 `internal` 包。
8. `core` 不允许导入 `modules/rulary-*`。
9. 所有写 Action 声明 Permission 与 Audit Level。
10. 要求 Preview 的 Action 实现 Plan。

### 6.3 F0 不实现的 Module 能力

- 版本范围求解。
- Optional dependency。
- Auto-install bridge。
- Module signature。
- 远程下载。
- License policy engine。
- Runtime enable / disable。
- Go `.so`。
- WASM 或子进程插件。

---

## 7. 静态组合与构建

### 7.1 项目清单

应用根目录：

```yaml
# modary.yaml
app:
  id: rulary-f0

modules:
  - database-sqlite
  - identity-local
  - authz-basic
  - audit
  - console-react
  - agent-mcp
  - rulary-core
```

### 7.2 生成结果

`modary build` 生成：

```text
internal/generated/modules_gen.go
web/src/generated/routes.ts
web/src/generated/navigation.ts
internal/generated/action_catalog.json
internal/generated/module_graph.json
```

构建结果必须可重复。相同输入清单不应产生无意义 Diff。

### 7.3 部署产物

```text
dist/modary-rulary
data/modary.db
```

生产环境不要求 Node.js。React 静态资源通过 `go:embed` 进入二进制。

---

## 8. F0 Action Contract

### 8.1 Descriptor

```go
type Descriptor struct {
    ID           string
    InputSchema  json.RawMessage
    OutputSchema json.RawMessage
    Permission   string
    Preview      PreviewPolicy
    AuditLevel   string
}
```

### 8.2 Handler

```go
type Handler interface {
    Plan(ctx context.Context, req Request) (Plan, error)
    Execute(ctx context.Context, plan Plan) (Result, error)
}
```

对于不需要 Preview 的简单 Action，可以使用 Adapter 包装成同一接口。

### 8.3 Request

```go
type Request struct {
    RequestID     string
    Actor         Actor
    Channel       string
    ActionID      string
    WorkspaceID   string
    Input         json.RawMessage
    IdempotencyKey string
    PlanHash      string
}
```

### 8.4 Runtime 流水线

```text
Resolve Action
→ Validate Input
→ Resolve Actor
→ Authorize
→ Build Plan
→ Enforce Limits
→ Return Preview or Verify Plan Hash
→ Execute
→ Record Audit
→ Return Typed Result
```

禁止 Channel Adapter 直接调用 Rulary Service。

---

## 9. F0 Action Catalog

| Action ID | 类型 | Permission | Preview | 允许入口 |
|---|---|---|---|---|
| `rulary.ruleset.create` | Write | `rulary.ruleset.create` | 否 | UI / API / CLI |
| `rulary.ruleset.update_draft` | Write | `rulary.ruleset.edit` | 否 | UI / API |
| `rulary.ruleset.validate` | Write | `rulary.ruleset.preview` | 否 | UI / API / Agent |
| `rulary.ruleset.preview` | Read-like | `rulary.ruleset.preview` | 自身即 Preview | UI / API / Agent |
| `rulary.ruleset.publish` | Write | `rulary.ruleset.publish` | 显示版本 Diff | UI / API |
| `rulary.run.execute` | High-risk Write | `rulary.run.execute` | **必须** | UI / API / Agent |
| `audit.query` | Read | `audit.read` | 否 | UI / API |

同一个 Action ID 在所有入口中映射到同一 Descriptor 与 Handler。

---

## 10. `rulary.run.execute` 设计

### 10.1 Preview 输入

```json
{
  "ruleset_version_id": "rv_001",
  "source": {
    "table": "company_license"
  },
  "target": {
    "table": "company_address_labels"
  },
  "limit": 100
}
```

### 10.2 Preview 输出

```json
{
  "plan_hash": "sha256:...",
  "ruleset_version": "1",
  "matched_rows": 83,
  "writable_rows": 47,
  "rejected_rows": 4,
  "unchanged_rows": 32,
  "target_table": "company_address_labels",
  "sample_results": [],
  "warnings": [],
  "expires_at": "2026-07-29T18:00:00Z"
}
```

### 10.3 Execute

Execute 必须携带：

```text
plan_hash
idempotency_key
```

以下情况必须拒绝并要求重新 Preview：

- RuleVersion 变化。
- 输入范围变化。
- Source 数据快照哈希变化，且变化会影响计划。
- Actor 或 Grant 变化。
- Plan 过期。
- 影响行数超过当前限制。
- 目标表配置变化。

### 10.4 事务

F0 中 Source、Target 与系统表位于同一个 SQLite 文件。结果写入、Run 状态和成功审计尽量在同一事务边界内完成。

失败审计可以在事务回滚后以独立 append-only 记录保存。

---

## 11. Rulary 最小 RuleSpec

F0 不建设完整通用规则 DSL，只保留足以验证版本、校验和执行的最小结构：

```json
{
  "schema_version": "rulary.ruleset.f0",
  "id": "company-address",
  "name": "企业地址标签",
  "source": {
    "table": "company_license",
    "primary_key": "company_id",
    "field": "license_address"
  },
  "operator": {
    "type": "rulary.address.extract_v1",
    "filing_marker": "经营地址备案",
    "parenthetical_note_target": "address_note"
  },
  "output": {
    "table": "company_address_labels",
    "unique_key": "company_id"
  }
}
```

`rulary.address.extract_v1` 属于 Rulary Module，不属于 Modary Core。

选择领域 Operator 是有意取舍：

- F0 验证平台的 Module 与 Action 合同。
- 不在首版顺手发明完整 Rule DSL。
- 后续 Rulary 可以将 Operator 演进为 Typed AST、SQL、Go 与 AI 混合执行，不影响 Modary。

---

## 12. 最小数据模型

### 12.1 平台表

```text
modary_user
modary_session
modary_role_binding
modary_agent_grant
modary_action_idempotency
modary_audit_log
modary_module_migration
```

### 12.2 Rulary 表

```text
rulary_workspace
rulary_ruleset
rulary_ruleset_version
rulary_run
rulary_label_result
```

### 12.3 测试业务表

```text
company_license
company_address_labels
```

### 12.4 跨表边界

- `authz-basic` 读取平台用户、角色和 Agent Grant。
- `rulary-core` 不直接修改身份或角色表。
- `audit` 拥有审计表。
- Modary Core 不直接查询 Rulary 领域表。
- Rulary 通过 Runtime Hook 产生审计事件，不写 `modary_audit_log`。

---

## 13. F0 权限模型

### 13.1 角色

| 角色 | 允许动作 |
|---|---|
| `rulary_author` | create、edit、validate、preview |
| `rulary_publisher` | author 权限 + publish |
| `rulary_operator` | preview、execute |
| `rulary_auditor` | audit.query |
| `workspace_admin` | F0 工作区全部权限 |

### 13.2 数据范围

F0 只实现一个简单范围条件：

```text
request.workspace_id == actor.workspace_id
```

Action Handler 可以增加领域前置条件：

```text
只有 draft 可以更新
只有已验证版本可以发布
只有 published 版本可以执行
```

权限与业务前置条件必须返回不同错误码：

```text
AUTHZ_DENIED
PRECONDITION_FAILED
VALIDATION_FAILED
PLAN_STALE
LIMIT_EXCEEDED
```

### 13.3 为什么 F0 不做完整 ABAC

F0 需要先证明 Authorizer 接口与 Runtime 强制点正确。

复杂 ABAC、ReBAC、字段 Mask、SQL Policy Compiler 和策略管理 UI 都可以以后实现为新 Authorizer，而不应改变 Action Handler。

---

## 14. F0 Agent Grant

最小 Grant：

```json
{
  "agent_id": "agent_rulary_operator",
  "delegated_by": "user_001",
  "workspace_id": "ws_default",
  "actions": [
    "rulary.ruleset.validate",
    "rulary.ruleset.preview",
    "rulary.run.execute"
  ],
  "max_rows": 50,
  "expires_at": "2026-07-30T00:00:00Z"
}
```

校验顺序：

```text
Agent 是否有效
→ Grant 是否过期
→ Action 是否在 allowlist
→ Delegator 是否仍拥有该权限
→ Workspace 是否匹配
→ Preview 影响是否 <= max_rows
→ Plan Hash 是否有效
```

F0 不支持 Grant 级金额表达式、关系图、多级转委托或复杂审批。

---

## 15. HTTP Action Gateway

F0 使用通用 Action Gateway：

```text
GET  /api/actions
GET  /api/actions/:id/schema
POST /api/actions/:id/preview
POST /api/actions/:id/execute
GET  /api/audit
```

认证接口：

```text
POST /api/auth/login
POST /api/auth/logout
GET  /api/auth/session
```

约束：

- React Console 也必须调用这些 Action API。
- Agent MCP Adapter 也必须构造同一 Action Request。
- 不为 Rulary 再实现一套绕过 Runtime 的 REST Controller。
- OpenAPI 从 Action Descriptor 派生或同步生成。
- HTTP 状态码不能替代结构化业务错误码。

---

## 16. React Console 范围

### 16.1 平台 Shell

- 登录。
- 左侧导航。
- 当前用户与角色。
- Module Route 注册。
- 通用 Action Confirm / Preview Dialog。
- 结构化错误展示。
- Audit 页面。

### 16.2 Rulary 页面

- RuleSet 列表。
- 创建 RuleSet。
- 草稿编辑页。
- Validate 结果。
- Preview 数据表格。
- 发布版本确认。
- 手动运行 Preview 与 Execute。
- Run 详情和单条结果追溯。

### 16.3 UI 非目标

- 通用拖拽页面设计器。
- 完整 Schema Form Engine。
- Chat 面板。
- 决策表。
- 数据连接向导。
- 调度中心。
- 通用审批中心。
- 可视化 Policy Studio。

Rulary 页面可以是普通 React 代码。F0 不要求所有业务 UI 都由 Manifest 自动生成。

---

## 17. CLI 范围

F0 CLI 只保留：

```bash
modary verify
modary generate
modary build
modary action catalog
modary action run <action-id>
```

### 17.1 Module 组合

F0 应用在 monorepo 根目录通过 `modary.yaml` 选择 Module，并通过
`modary verify` 与 `modary generate` 验证和生成静态组合。独立项目脚手架、
本地路径依赖、远程市场、版本搜索和下载不属于 F0 CLI 合同。

### 17.2 `modary action run`

用于开发和验收：

```bash
modary action run rulary.ruleset.preview \
  --actor user_001 \
  --input preview.json
```

它必须调用同一个 Runtime，不直接实例化 Handler。

---

## 18. 工程目录建议

```text
modary/
├── cmd/
│   ├── modary/
│   └── rulary-f0/
├── core/
│   ├── module/
│   ├── action/
│   ├── authz/
│   ├── audit/
│   └── migration/
├── modules/
│   ├── database-sqlite/
│   ├── identity-local/
│   ├── authz-basic/
│   ├── audit/
│   ├── console-react/
│   ├── agent-mcp/
│   └── rulary-core/
├── internal/
│   └── generated/
├── web/
├── examples/
│   └── rulary-f0/
├── tests/
│   ├── contract/
│   ├── integration/
│   └── e2e/
├── modary.yaml
├── SSOT.md
└── docs/
    ├── milestones/F0-RULARY.md
    └── adr/
```

边界检查必须保证 `core/` 不依赖 `modules/`。

---

## 19. 技术选型

### 19.1 Backend

```text
Go
标准 context / error / net/http 合同
HTTP Router：Chi 或 GoFrame Runtime Adapter 二选一原型
SQLite：database/sql
Migration：Goose 或等价薄封装
Logging：log/slog
Schema：JSON Schema
```

GoFrame 可以提供 HTTP、配置、日志、错误处理或 CLI 体验参考，但不得让 `ghttp.Request`、`gdb.Model`、`g.Map` 等类型进入 `core/action` 与 `core/module` 公共接口。

### 19.2 Frontend

```text
React
TypeScript
Vite
TanStack Query
TanStack Table
React Hook Form
轻量自有组件 / Radix 风格基础组件
```

### 19.3 Packaging

```text
Vite build
→ go:embed
→ single Go binary
```

---

## 20. 非功能要求

### NFR-001：部署

F0 必须可以在 2C2G Linux 服务器上，以一个进程和一个 SQLite 文件运行。

### NFR-002：依赖

生产运行不得强制需要：

```text
Node.js
Redis
PostgreSQL
Kafka
RabbitMQ
Temporal
MinIO
Kubernetes
```

### NFR-003：资源预算

参考 Linux amd64 Release 构建：

- 空载 RSS 工程预算：不高于 128 MiB。
- 冷启动到 Readiness：不高于 2 秒。
- 资源预算是 F0 验收指标；若未达到，必须提供 profile 与原因，不能静默放宽。

### NFR-004：Preview 性能

在 2C2G 参考环境、1000 条地址样例、本地 SQLite、无外部 AI 调用的情况下：

- Preview 目标：P95 不高于 3 秒。
- UI 首屏不等待全部 Preview 完成。
- 大结果仅返回摘要与分页样本。

### NFR-005：确定性

相同 Module 清单执行两次生成，不应产生语义无关 Diff。

### NFR-006：安全

- Session 使用 Secure、HttpOnly Cookie。
- 密码使用成熟 KDF。
- 所有写 Action 默认 CSRF 防护。
- Agent 使用独立凭证与 Grant。
- 输入与日志中的敏感字段可脱敏。
- 缺失授权信息时 Fail Closed。

### NFR-007：审计

成功、拒绝和失败 Action 都必须产生可关联的审计事件。

### NFR-008：可测试

核心 Runtime 测试不依赖浏览器、网络或完整 Rulary UI。

---

## 21. 测试策略

### 21.1 Contract Tests

- Module Manifest 解析。
- Missing capability。
- Duplicate provider。
- Circular dependency。
- Duplicate Action ID。
- Action input validation。
- Authorizer deny。
- Plan hash stability。
- Plan stale。
- Idempotency replay。
- Audit hook。

### 21.2 Module Boundary Tests

- `core` 不导入 `modules`。
- Rulary 不写身份表。
- Channel Adapter 不导入 Rulary internal service。
- Module 只能访问声明能力。

### 21.3 Rulary Golden Tests

至少包含：

- 经营地址备案。
- 普通括号方位说明。
- 没有经营地址。
- 多个括号。
- 空地址。
- 全角 / 半角括号。
- 分号与中文分号。
- 证据必须来自原文。
- 同一输入重复执行幂等。
- 已发布版本不可修改。

### 21.4 Cross-channel Tests

对同一个 `rulary.run.execute`：

```text
UI
HTTP
CLI
Agent
```

必须验证：

- Action ID 相同。
- Handler 相同。
- Permission 相同。
- Plan Hash 语义相同。
- Audit Schema 相同。
- 输出业务结果相同。

### 21.5 E2E

浏览器自动化覆盖：

```text
login
→ create ruleset
→ validate
→ preview
→ publish
→ run preview
→ confirm execute
→ inspect result
→ inspect audit
```

---

## 22. F0 验收标准

F0 只有全部满足以下条目才算通过。

### AC-001：模块组合

从 `modary.yaml` 生成静态 Registry，应用成功启动；移除 `agent-mcp` 后 UI/API 仍然工作。

### AC-002：依赖验证

故意删除 `database-sqlite` 时，`modary verify` 在构建前给出可理解的缺失依赖错误。

### AC-003：循环依赖

构造两个测试 Module 的循环依赖，构建必须失败并展示完整路径。

### AC-004：跨入口统一 Action

UI、API、CLI 与 Agent 对 Rulary Preview/Execute 使用同一个 Action Descriptor 和 Handler。

### AC-005：权限不可绕过

`rulary_author` 可以 Preview 但不能 Publish 或 Execute；直接调用 HTTP 或 MCP 也必须被拒绝。

### AC-006：结构化拒绝解释

拒绝结果至少包含：

```text
error_code
action_id
required_permission
actor_id
workspace_id
human_readable_reason
request_id
```

### AC-007：Preview / Execute 一致

Execute 必须验证有效 `plan_hash`；修改规则版本或影响范围后，旧 Plan 失效。

### AC-008：Agent 上限

Agent Grant 的 `max_rows = 50` 时，影响 51 行的 Execute 必须拒绝。

### AC-009：Rulary 地址结果

核心复杂地址样例输出与预期完全一致，证据片段可在原文找到。

### AC-010：版本不可变

Published RuleVersion 不可原地修改；修改必须创建新版本。

### AC-011：幂等

相同 `idempotency_key` 重试不得重复写入结果或创建第二次业务副作用。

### AC-012：统一审计

每次 Action 记录 actor、channel、action、decision、plan hash 与结果摘要；不同入口只在 channel 等上下文上不同。

### AC-013：单二进制

Release 产物在没有 Node、Redis、PostgreSQL 等服务的干净 2C2G 环境启动并完成全流程。

### AC-014：资源预算

满足 NFR-003，或在验收报告中明确失败并阻止 F0 标记为完成。

### AC-015：Kernel 无领域泄漏

静态依赖检查确认 Modary Core 不包含 `RuleSet`、`RuleSpec`、`Label`、`Fluxale Spec` 等领域概念。

---

## 23. F0 Cut Order

范围失控时按以下顺序削减：

1. Agent Execute 降为 Agent Preview；保留同一 Action 与权限验证。
2. 删除 `modary action run` 的高级输出，只保留基本 JSON。
3. UI 只保留一个 RuleSet 详情工作台，不做完整列表筛选。
4. Agent MCP 只暴露 `validate`、`preview` 与带 Plan Hash 的 `execute`。
5. Module UI Contribution 只支持 Route，不做 Slot、Toolbar、Field Renderer。
6. 地址 Operator 只覆盖 Golden Dataset，不扩展成通用规则语言。

以下内容**不可裁剪**：

```text
静态模块验证
同一 Action Runtime
Authorize
Preview / Plan Hash
Audit
Rulary 真实纵向切片
单二进制部署
```

若必须裁剪这些内容，说明 F0 已经失去验证价值，应停止而不是伪装完成。

---

## 24. 工程阶段

### F0-A：Kernel Skeleton

交付：

- Module Manifest。
- Dependency verifier。
- Static registry generator。
- Action registry 与 Runtime。
- Authorizer / Audit interfaces。
- 单元测试。

Gate：

```text
两个测试 Module 可以组合
缺失依赖和循环依赖会失败
一个示例 Action 可从 CLI 执行
```

### F0-B：Base Modules

交付：

- SQLite。
- Local Identity。
- Basic Authz。
- Audit。
- React Shell。
- Generic Action Gateway。

Gate：

```text
登录
→ 调用受保护 Action
→ 权限拒绝
→ 查看审计
```

### F0-C：Rulary Vertical Slice

交付：

- RuleSet / Version / Run。
- Address Operator。
- Validate / Preview / Publish / Execute。
- Rulary React 页面。
- Golden Tests。

Gate：

```text
核心地址样例端到端通过
```

### F0-D：Agent 与交付验证

交付：

- Agent Grant。
- MCP Adapter。
- Cross-channel tests。
- 单二进制 Release。
- 2C2G benchmark。
- F0 验收报告。

Gate：

```text
所有 AC 通过
```

---

## 25. Definition of Done

F0 标记完成前必须存在：

- 可运行 Release 二进制。
- 可复现构建命令。
- F0 Module graph。
- Action catalog。
- OpenAPI 或 Action Schema 文档。
- Golden Dataset。
- Contract、Integration、E2E 测试。
- 资源基准报告。
- 权限矩阵。
- 审计样例。
- 已知限制清单。
- 下一阶段提案，但不把 F1 内容混入 F0。

---

## 26. F0 成功后的下一步选择

F0 完成后只允许选择一个主方向：

### 选项 A：继续完善 Rulary

触发条件：

- Rulary 使用平台明显降低重复基础设施开发。
- RuleOps 产品验证优先级更高。
- Rulary 用户试点可快速获得。

### 选项 B：第二个非 Rulary 应用验证

触发条件：

- 需要验证 Kernel 是否真的领域无关。
- Rulary 定制代码过多，无法判断哪些能力可复用。
- Fluxale Control Plane 或另一个小型业务系统已具备真实需求。

### 选项 C：优先深化授权模块

触发条件：

- 两个应用都出现相同的 RBAC + 数据范围 + Agent Delegation 痛点。
- `authz-basic` 合同经过真实使用稳定。
- 高级授权可以不修改 Action Runtime。

F0 完成前不选择以上方向。

---

## 27. 已确认决策

### F0-DR-001：Rulary 是唯一验收对象

- 状态：Accepted
- 原因：避免同时做平台、Fluxale 和权限产品。

### F0-DR-002：只实现确定性地址 Operator

- 状态：Accepted
- 原因：验证平台不需要先完成完整 AI RuleOps。

### F0-DR-003：RBAC + Workspace Scope

- 状态：Accepted
- 原因：先验证 Authorizer 强制点，再扩展 ABAC。

### F0-DR-004：Agent 使用 Action Allowlist + maxRows

- 状态：Accepted
- 原因：足够证明 Agent 无法绕过 Runtime，又不建设完整 Delegation 平台。

### F0-DR-005：SQLite 单机

- 状态：Accepted
- 原因：符合轻量目标，减少数据库与队列变量。

### F0-DR-006：普通 React 页面优先

- 状态：Accepted
- 原因：F0 不验证通用低代码 Renderer，业务 UI 可以由 Module 自己拥有。

---

## 28. 开放问题

以下问题允许在实现中通过 ADR 收敛：

1. F0 HTTP Runtime 使用 Chi 还是 GoFrame Adapter。
2. SQLite 驱动使用纯 Go 还是 CGO。
3. JSON Schema Validator 的具体实现。
4. Plan 持久化到数据库还是短期签名 Token。
5. React UI Contribution 使用构建期生成 import，还是统一 Route Registry。
6. MCP Adapter 的具体 SDK。
7. 密码 KDF 使用 Argon2id 还是 scrypt。
8. Audit 失败事件与业务事务的最终写入边界。
9. Preview Source Snapshot Hash 的最低成本算法。

这些问题不能改变 F0 的 Action Runtime 与验收标准。

---

## 29. User Review Gate

- F0 objective：Accepted
- Acceptance object：Rulary，已指定
- Scope：Accepted
- Architecture implementation：Accepted，见 `docs/adr/ADR-007-f0-runtime-implementation.md`
- Development：Complete
- Completion authority：本文件 AC-001 至 AC-015
