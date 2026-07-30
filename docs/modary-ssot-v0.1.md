# Modary SSOT

> 项目名称：**Modary（工作名）**
> 中文定位：**面向人类与 Agent 的轻量模块化业务应用内核**
> 英文定位：**A lightweight modular application kernel for humans and agents**
> 文档类型：Project Constitution / Long-term Single Source of Truth
> 版本：v0.1
> 日期：2026-07-30
> 状态：Working SSOT / 核心产品合同已确认，F0 验收通过
> 口号：**Modules in. Governed actions out.**
> 中文表达：**按需组合模块，统一治理动作。**

---

## 0. 文档权威与使用方式

本文件记录 Modary 即使在实现细节变化后仍应保持稳定的产品真相、核心模型、边界和长期原则。

本文件**不承担当前版本的详细交付范围**。当前工程范围由对应里程碑合同定义：

```text
SSOT.md                         项目宪法：为什么做、核心是什么、哪些原则不能破坏
docs/modary-f0-rulary-v0.1.md  当前交付合同：这一版做什么、如何验收、先砍什么
docs/adr/*.md                  技术决策记录：为什么选择某个实现
代码与测试                     对已批准合同的可执行实现
```

优先级规则：

1. 里程碑合同不得违反本 SSOT 的核心原则。
2. 同一里程碑范围内，以该里程碑合同为准。
3. 技术实现变化写入 ADR，不反向膨胀本 SSOT。
4. 任何需要增加 Kernel 核心概念的提案，必须更新本 SSOT 并单独评审。
5. 未写入本文件的长期设想不构成承诺。

---

## 1. 一句话定义

**Modary 是一个轻量模块化业务应用内核：业务模块贡献类型化 Action，UI、API、CLI、任务与 AI Agent 通过同一个受控 Runtime 执行这些 Action。**

产品公式：

```text
Module + Action + Execute
```

展开后是：

```text
Modules provide typed Actions
Callers invoke Actions
Runtime governs execution
```

Modary 的目标不是替开发者重新发明 Web、ORM、前端框架或消息队列，而是解决两个更高层的问题：

1. 多个完整业务能力如何以可验证、可裁剪的方式组合成一个应用。
2. 人类、服务与 Agent 如何在不复制业务逻辑、不绕过权限的前提下执行真实业务动作。

---

## 2. 核心问题

### 2.1 传统后台模板的问题

传统后台框架通常围绕以下流水线展开：

```text
数据库表
→ Model
→ Service
→ Controller
→ REST API
→ 前端 API
→ 表格 / 表单
→ 菜单
→ 角色权限
```

代码生成可以加速第一次创建，却容易在后续变化中形成多个事实源：

- 数据库 Schema。
- 后端模型。
- API 定义。
- 前端类型。
- 页面表单。
- 菜单与权限。
- Agent Tool。
- 工作流节点。

AI 可以更快地产生这些代码，但不会自动消除漂移、重复实现和权限绕行。

### 2.2 大而全平台的问题

很多插件化或低代码平台通过运行时微内核、动态插件、元数据数据库和大量基础设施获得灵活性，但代价通常是：

- 默认部署依赖过多。
- 模块边界依赖约定而非机器验证。
- 插件可以修改过多全局状态。
- 升级和卸载难以预测。
- 小服务器与私有化试用门槛高。
- 框架逐渐成为兼容层，而不是稳定内核。

### 2.3 Agent 时代的新问题

如果 UI、API、工作流和 Agent 分别拥有自己的业务入口，就会出现：

```text
UI 有权限校验
API 有另一套权限校验
Agent 直接调用底层 Service
批量任务绕开预览
审计只记录部分入口
```

Modary 的核心要求是：

> **业务动作只实现一次；所有调用方必须经过同一个执行合同。**

---

## 3. 北极星目标

### 3.1 产品北极星

开发者可以按需组合业务模块，并获得一个仍然可理解、可测试、可部署的应用：

```text
选择模块
→ 验证依赖
→ 静态组合
→ 构建单一应用
→ 所有入口共享 Action Runtime
```

### 3.2 开发体验北极星

一个新业务能力应优先表达为：

```text
一个 Module
+ 一组 Actions
+ 可选 UI / Migration / Configuration 贡献
```

开发者不需要为了新增一个业务能力，同时手写多套 Controller、Agent Tool、CLI 命令和审计逻辑。

### 3.3 运行体验北极星

默认部署应尽可能接近：

```text
一个 Go 二进制
+ 一个 SQLite 文件
+ 一个数据目录
```

高级能力可以替换，但不得成为所有用户的强制成本。

### 3.4 治理北极星

任何重要写操作都能够回答：

1. 谁或哪个 Agent 发起了动作。
2. 从哪个入口发起。
3. 调用了哪个 Action。
4. 使用了什么权限和约束。
5. 预期影响是什么。
6. 最终修改了什么。
7. 为什么被允许或拒绝。
8. 如何追溯结果。

---

## 4. 核心模型：两个名词，一个动词

Modary 刻意把 Kernel 的核心模型压缩为：

```text
Module
Action
Execute
```

### 4.1 Module

Module 是应用的唯一组合单位。

Module 可以贡献：

- Actions。
- Services。
- Database migrations。
- Configuration schema。
- UI routes、navigation 与 components。
- 可选的后台任务或协议适配器。

首版与核心合同只区分两类 Module：

| 类型 | 含义 | 示例 |
|---|---|---|
| Feature Module | 提供完整业务能力 | audit、rulary-core、billing |
| Adapter Module | 提供某种基础设施实现 | database-sqlite、identity-oidc、storage-s3 |

以下概念不进入 Kernel 的独立类型体系：

- Bridge Module：本质是同时依赖两个能力的 Feature Module。
- Pack：本质是一份模块组合清单。
- Extension Module：本质是扩展某 Feature 的普通 Module。
- Isolated Plugin：是未来的部署与安全机制，不是首版核心模型。

### 4.2 Action

Action 是系统能够执行的类型化业务行为。

示例：

```text
rulary.ruleset.create
rulary.ruleset.preview
rulary.ruleset.publish
rulary.run.execute
user.disable
audit.export
```

Action 至少声明：

- 稳定 ID。
- 输入 Schema。
- 输出 Schema。
- 权限标识。
- 是否支持或要求 Preview。
- Handler。
- 审计级别。

Action 不等同于 HTTP Endpoint，也不等同于按钮。HTTP、UI、CLI、任务和 Agent 都只是 Action 的调用入口。

### 4.3 Execute

Execute 是 Kernel 的核心动词。

标准流水线：

```text
Validate
→ Authorize
→ Preview / Plan
→ Execute
→ Audit
```

只读动作可以跳过 Preview；高影响写动作必须支持 Preview。任何入口不得绕过 Validate、Authorize 和 Audit。

---

## 5. 核心承诺

### 5.1 只安装需要的能力

项目不以“内置功能数量”竞争。默认安装应小，模块按需加入。

### 5.2 静态组合优先

官方和可信模块默认在构建期完成依赖解析与静态注册，最终编译为一个应用。

不以 Go `.so`、运行时反射扫描或任意脚本注入作为主扩展机制。

### 5.3 业务动作只实现一次

同一个 Action 可以被多个通道调用，但只能有一个权威业务实现。

### 5.4 人与 Agent 同路不同权

人类、服务账号和 Agent 都是调用方。它们经过同一个 Runtime，但可以拥有不同的授权范围、过期时间与影响上限。

### 5.5 默认轻量，高级能力可替换

SQLite、本地 Session、数据库任务表与本地存储可以作为默认实现；PostgreSQL、OIDC、外部授权、S3、分布式队列可以作为 Adapter。

### 5.6 组合结果可解释

模块为什么被启用、依赖由谁满足、Action 从何注册、权限为何拒绝，都应当能被工具解释。

### 5.7 失败时保守

缺少依赖、权限属性、Preview 一致性或迁移前置条件时，系统默认拒绝构建或执行，而不是猜测。

---

## 6. Module 合同

### 6.1 最小 Manifest

长期合同保持简单：

```yaml
schemaVersion: modary.module/v1

id: audit
version: 0.1.0
type: feature

requires:
  - database
  - identity

provides:
  - audit

actions:
  - audit.query
  - audit.export

migrations:
  - migrations

ui:
  routes:
    - id: audit.logs
      path: /settings/audit
```

Manifest 描述组合事实，不承担任意编程逻辑。

### 6.2 依赖原则

Module 依赖能力而非某个固定产品：

```yaml
requires:
  - database
  - identity
  - authorization
```

而不是：

```yaml
requires:
  - postgres
  - casdoor
  - casbin
```

首版只需要支持简单能力键与唯一提供者检查。复杂版本求解、条件依赖和远程 Registry 在真实需求出现后再引入。

### 6.3 边界原则

每个 Module 必须：

1. 拥有自己的内部代码与数据表。
2. 只通过公开 Action、Service 或 Event 合同与其他模块交互。
3. 不直接导入其他模块的 `internal` 包。
4. 不直接写入其他模块拥有的数据表。
5. 显式声明依赖与提供能力。
6. 可以在测试中独立启动或使用替代 Adapter。

### 6.4 数据所有权

表名、迁移和数据生命周期必须有明确 Module Owner。

删除 Module 默认只移除注册与代码，不自动销毁业务数据。破坏性清理必须是显式、可审计的独立操作。

### 6.5 静态注册

构建工具生成显式注册代码：

```go
func RegisterModules(host *Host) error {
    return host.Register(
        sqlite.Module(),
        identitylocal.Module(),
        authzbasic.Module(),
        audit.Module(),
        rulary.Module(),
    )
}
```

禁止以隐藏 `init()`、空白 import 或目录扫描作为主要注册方式。

---

## 7. Action 合同

### 7.1 最小 Descriptor

概念合同：

```go
type Descriptor struct {
    ID            string
    InputSchema   Schema
    OutputSchema  Schema
    Permission    string
    Preview       PreviewPolicy
    AuditLevel    AuditLevel
}
```

Handler 合同：

```go
type Handler interface {
    Plan(ctx context.Context, req Request) (Plan, error)
    Execute(ctx context.Context, plan Plan) (Result, error)
}
```

只读 Action 可以使用统一的简化 Handler，但外部仍通过同一 Runtime 调用。

### 7.2 Execution Request

调用请求至少包含：

```text
request_id
actor
channel
action_id
input
workspace / tenant context
idempotency_key
optional preview token / plan hash
```

Actor 是执行上下文中的数据，不需要膨胀为 Kernel 领域模型。

### 7.3 Preview 与一致性

对需要 Preview 的动作：

```text
输入
→ 生成 Plan
→ 返回影响摘要和 plan_hash
→ 用户或调用方确认
→ 使用同一 plan_hash 执行
```

若关键数据、权限、版本或输入发生变化，旧 Plan 必须失效或重新预览。

### 7.4 审计

Audit 是 Runtime Hook，不要求 Kernel 理解每个业务领域。

最小审计事件包括：

```text
request_id
actor_id / actor_type
channel
action_id
input_hash
plan_hash
decision
result_summary
error_code
started_at
finished_at
```

敏感输入不得默认完整写入日志。

### 7.5 幂等

有外部副作用或批量写入的 Action 必须支持幂等键。幂等语义属于 Action 合同，不由 HTTP 重试策略隐式决定。

---

## 8. Channel Adapter

以下都只是 Action Runtime 的入口适配器：

```text
React Console
HTTP / OpenAPI
CLI
Scheduled Job
Workflow
MCP / Agent
```

Adapter 负责：

- 解析调用方身份。
- 校验传输层格式。
- 构造统一 Action Request。
- 展示 Plan、Result 和结构化错误。

Adapter 不得：

- 直接调用 Module 内部 Service 绕过 Runtime。
- 自己复制业务权限。
- 自己实现另一份业务事务。
- 为 Agent 提供比 Action 更底层的任意数据库能力。

---

## 9. 身份、授权与 Agent 边界

### 9.1 身份与授权分离

Identity 回答“调用者是谁”；Authorization 回答“此调用者能否执行此 Action”。

Casdoor、Keycloak、Authentik 或本地账号可以作为 Identity Adapter，但业务授权不应绑定某一身份产品。

### 9.2 Kernel 只定义 Authorizer 合同

```go
type Authorizer interface {
    Authorize(ctx context.Context, req AuthorizationRequest) (Decision, error)
}
```

Kernel 不内置完整 RBAC、ABAC、ReBAC 或 Policy Studio。

官方实现可以演进为：

```text
authz-basic        RBAC + 简单范围
authz-abac         属性条件
authz-relation     关系权限
authz-delegation   临时委托
authz-studio       策略管理界面
```

它们都实现同一个 Authorizer 合同。

### 9.3 Agent 原则

Agent 不是超级管理员，也不是拿着用户完整 Token 的自动脚本。

最小约束可以包括：

```text
允许的 Action
工作区范围
过期时间
最大影响行数
委托人
```

Agent 不能通过委托获得委托人本身不具备的权限。

### 9.4 长期授权方向

长期可以组合：

```text
RBAC：拥有什么能力
ReBAC：在哪些对象范围内生效
ABAC：什么条件下生效
Delegation：本次委托最多能做什么
Obligation：执行前后还必须满足什么
```

但这些是可演进的授权模块，不得反向增加 Kernel 核心概念。

---

## 10. 总体架构

```text
┌──────────────────────────────────────────────┐
│ Channel Adapters                             │
│ React │ HTTP │ CLI │ Job │ Workflow │ Agent │
└──────────────────────┬───────────────────────┘
                       │ Action Request
┌──────────────────────▼───────────────────────┐
│ Action Runtime                               │
│ Validate → Authorize → Plan → Execute → Audit│
└──────────────┬──────────────────┬────────────┘
               │                  │
┌──────────────▼──────┐  ┌────────▼────────────┐
│ Feature Modules     │  │ Adapter Modules     │
│ Rulary / Audit ...  │  │ DB / Identity ...  │
└─────────────────────┘  └─────────────────────┘
```

### 10.1 Kernel 必须保持小

Kernel 只拥有：

- Module graph 与静态注册合同。
- Action registry。
- Action Runtime。
- Authorizer 接口。
- Audit Hook。
- Module migration coordination。
- 构建期验证与最小诊断能力。

Kernel 不拥有：

- 通用 CRUD 领域模型。
- 工作流引擎。
- 页面搭建器。
- Rulary RuleSpec。
- Fluxale Spec。
- 具体 IAM 产品。
- 具体数据库 ORM。
- 具体 Agent SDK。

### 10.2 Service 组合

F0 及早期版本优先生成显式构造代码，而不是实现大型运行时 DI 容器。

依赖图在构建期验证，服务对象在启动时显式传递。

---

## 11. 技术基线

### 11.1 默认技术形态

```text
Backend: Go
Frontend: React + TypeScript + Vite
Default database: SQLite
Optional database: PostgreSQL
Packaging: go:embed single binary
Production Node.js runtime: not required
```

### 11.2 GoFrame 的借鉴边界

GoFrame 值得借鉴：

- Core 与 Contrib 分离。
- 接口优先并提供默认实现。
- CLI 作为工程规范入口。
- 统一错误、日志、配置和生成工具体验。
- 可按需使用基础组件。

Modary 不复制 GoFrame，也不让 GoFrame 类型进入公共合同。GoFrame、标准库、Chi 或其他基础设施都可以作为 Runtime 实现；Modary 的核心价值位于 Module、Action 与 Execute 层。

### 11.3 默认不强制

```text
Redis
Kafka
RabbitMQ
Temporal
Kubernetes
MinIO
Elasticsearch
Node.js SSR
本地大模型
```

真实需求出现后，可以通过 Adapter 或独立部署模式增加。

---

## 12. 目标用户

### 12.1 Primary

- 需要快速构建内部业务系统的独立开发者和小团队。
- 需要在 2C2G 等小服务器上部署多个系统的开发者。
- 需要把业务后台同时暴露给人类与 Agent 的团队。
- 有明确垂直产品，但不想重复开发用户、审计、Action 与模块基础设施的团队。

### 12.2 Secondary

- 需要轻量私有化交付的传统企业软件团队。
- 希望构建可裁剪控制面的开源项目。
- 希望统一多个业务应用授权与 Agent 操作路径的平台团队。

### 12.3 非首版用户

- 需要全球多区域强一致授权的超大型组织。
- 依赖数百个运行时热插件的大型生态平台。
- 需要完整 ERP、CRM 或 BPM 套件的企业。
- 只想生成一次性 CRUD 页面、无需长期维护的用户。

---

## 13. 产品边界

Modary **不是**：

- 新的 Go Web Framework。
- 新的 ORM。
- Go 版 Spring 全家桶。
- 更现代的 gin-vue-admin 模板。
- 通用低代码页面搭建器。
- 通用微服务框架。
- 完整 IAM 产品。
- 通用 Agent 框架。
- 通用工作流平台。
- 首版即拥有插件市场的生态平台。

Modary 可以与上述工具结合，但不应重复它们的核心职责。

---

## 14. 与 Rulary 的关系

Rulary 是 Modary 的第一个真实 F0 验收对象。

关系如下：

```text
Modary
├── 提供 Module、Action、Execute、基础授权与审计
└── 不理解 RuleSpec、标签、地址或数据规则

Rulary Module
├── 拥有 RuleSet / RuleVersion / RuleRun
├── 拥有规则验证、预览与执行语义
├── 贡献 Rulary Actions
└── 贡献自己的 React 页面
```

Rulary 的领域概念不得进入 Modary Kernel。只有同时被 Rulary 与第二个非 Rulary 应用复用并证明稳定的机制，才有资格进入公共平台层。

Rulary F0 是对 Modary 的 dogfood，不等同于完整 Rulary 产品发布。Rulary 的 Chat-first、AI 规则协作、调度、跨数据库与完整 RuleOps 能力继续由 Rulary 自己的里程碑推进。

---

## 15. 与 Fluxale 的关系

Fluxale 可以在未来把用户、组织、DataSource、发布、Render Job、审计等 Control Plane 能力迁移到 Modary。

Fluxale Workbench Core、Fluxale Spec、画布事务、Selection、Surface、动画和渲染仍然属于 Fluxale，不进入 Modary Kernel。

可共享的是：

```text
Module composition
Action descriptor
Identity / authorization adapter
Audit
Jobs adapter
UI contribution contract
```

不可合并的是：

```text
Artifact transaction
Business database transaction
Fluxale Spec
Rulary RuleSpec
领域 Surface 与编辑模型
```

---

## 16. 长期演进方向

以下是可能的模块方向，不构成 F0 或固定路线承诺：

```text
resource-admin
jobs-db
workflow
identity-oidc
identity-casdoor
authz-abac
authz-relation
authz-delegation
storage-s3
agent-mcp
module-registry
source-owned-feature-kits
isolated-plugin-host
```

任何方向进入开发前必须证明：

1. 不能通过普通 Feature Module 解决。
2. 不会增加所有用户的默认成本。
3. 不会破坏静态确定性。
4. 在至少一个真实应用中有明确需求。

---

## 17. 非功能原则

### 17.1 轻量

2C2G 是首要参考部署环境，而不是不受支持的边缘场景。

### 17.2 确定性

相同模块清单、版本与配置应产生可复现的依赖图、生成代码和构建结果。

### 17.3 可观察

Action 必须具备 request ID、结构化日志、审计事件和可定位错误。

### 17.4 可测试

Module、Action、Authorizer 和 Channel Adapter 都应可在不启动完整系统时测试。

### 17.5 安全默认值

- 默认最小权限。
- 默认不允许任意 SQL 或脚本。
- 默认不把 Secret 暴露给浏览器。
- 高影响 Action 默认要求 Preview。
- 缺失关键属性时默认拒绝。

### 17.6 可退出

业务模块必须允许开发者使用普通 Go 与 React 编写复杂页面和逻辑，不强迫所有功能进入通用元数据渲染器。

---

## 18. 成功标准

项目长期价值成立，需要逐步证明：

1. 不同垂直应用可以复用同一个小型 Kernel。
2. 一个业务 Action 能够被至少三个入口复用而不复制逻辑。
3. 模块安装、移除和升级产生确定、可审查的 Diff。
4. Agent 无法绕过正常权限、Preview 与审计。
5. 默认部署明显轻于大而全业务平台。
6. 使用 Modary 构建真实产品，比每次重写基础设施更简单。
7. Kernel 的领域概念没有随着 Rulary 或 Fluxale 膨胀。

---

## 19. 终止或重构条件

出现以下情况时，应停止扩张并重新评估定位：

1. Modary 最终只是一个 Go + React CRUD 模板。
2. Action 不能真正跨 UI、API 与 Agent 复用。
3. 为支持模块化而引入的复杂度高于业务收益。
4. Kernel 开始理解 Rulary、Fluxale 等具体领域。
5. 大量功能只能依赖运行时魔法和全局状态实现。
6. 轻量部署被默认 Redis、消息队列或多个服务破坏。
7. Rulary 之外没有第二个真实应用愿意复用平台。
8. 用户主要需求只是代码生成，现有 Agent 已能更好完成。

---

## 20. 品牌与产品体系

### 20.1 工作名

当前工作名：**Modary**。

命名意图：

```text
Module / Modular
+ Library / Assembly
→ Modary
```

它表达“按需组合模块形成应用”，且不绑定 Go、后台、低代码或 AI，允许产品长期扩展。

建议产品体系：

```text
Modary Core       核心合同与 Runtime
Modary CLI        创建、验证、构建
Modary Console    默认管理外壳
Modary Modules    官方模块集合
Modary Registry   未来模块分发服务
Modary Cloud      未来托管能力
```

### 20.2 品牌状态

Modary 当前仅作为工程工作名。公开发布前必须完成：

- GitHub 组织与仓库名核查。
- Go module、npm package 与容器镜像名核查。
- 主要软件类别商标初筛。
- 域名与社交账号核查。
- 中英文发音与搜索可发现性验证。

名称变化不得影响 Module、Action 与 Execute 的产品合同。

---

## 21. 已确认决策

### PDR-001：核心公式是 Module + Action + Execute

- 状态：Accepted
- 原因：它是可以解释项目差异、又足够小的最小模型。

### ADR-001：静态模块组合优先

- 状态：Accepted
- 决策：官方模块通过构建期验证与显式注册组合。
- 原因：轻量、确定、易调试、适合 Go 单二进制。

### ADR-002：Go + React/Vite

- 状态：Accepted
- 决策：Go 负责 Runtime，React/Vite 负责高交互管理界面。
- 原因：生产环境不需要 Node.js SSR，生态与资源占用平衡。

### ADR-003：SQLite-first

- 状态：Accepted
- 决策：默认 SQLite；高级环境可替换 PostgreSQL。
- 原因：降低试用、私有化和 2C2G 部署门槛。

### ADR-004：授权是接口，不是 Kernel 内置大平台

- 状态：Accepted
- 决策：Kernel 定义 Authorizer；RBAC、ABAC、ReBAC 与 Delegation 由模块演进。
- 原因：保留长期上限，同时避免首版膨胀。

### ADR-005：Rulary 是 F0 唯一验收对象

- 状态：Accepted
- 决策：使用 Rulary 地址标签纵向切片验证平台。
- 原因：真实业务比玩具 Todo 更能暴露模块、权限、Preview 与审计问题。

### ADR-006：GoFrame 只作为借鉴或 Runtime 实现

- 状态：Accepted
- 决策：借鉴其工程体验，但公共合同不依赖 GoFrame 类型。
- 原因：避免把产品价值绑定到底层框架。

---

## 22. 开放问题

1. Modary 是否作为最终品牌，需要完成正式品牌核查。
2. F1 是否优先验证第二个非 Rulary 应用，还是先完善 Rulary。
3. Module UI Contribution 的稳定最小合同应在 F0 实现后冻结。
4. Action 的 Plan 是否需要统一持久化，将由 F0 运行数据决定。
5. PostgreSQL Adapter 何时进入官方支持，由真实多实例需求决定。
6. 授权能力何时从 `authz-basic` 拆分为独立项目，必须先经过两个真实应用验证。
7. 是否使用 GoFrame 作为 F0 Runtime 实现，由原型对比决定，不属于产品层不可变决策。

---

## 23. User Review Gate

- Product direction：Confirmed
- Core model：Confirmed
- F0 acceptance object：Rulary
- Working name：Modary，待品牌核查
- F0 implementation：Pending
- 下一份权威文档：`Modary F0 — Rulary Vertical Slice`
