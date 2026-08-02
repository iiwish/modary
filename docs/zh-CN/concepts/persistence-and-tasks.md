# 持久化与耐久任务

Modary 明确区分控制数据、任务队列数据和业务数据。三者可以协作，但所有权和
事务边界不同。

## 控制数据库

官方 PostgreSQL Profile 使用一个控制数据库和两个 schema：

| 边界 | 默认 schema | 典型内容 |
| --- | --- | --- |
| 应用控制面 | `modary` | Module 迁移、Action plan、幂等结果、Identity、RBAC、Audit、应用控制表 |
| 耐久任务运行时 | `modary_queue` | Modary Profile 绑定信息，以及 River job、queue、leader、schedule 和迁移状态 |

两个 schema 名称可配置，但必须不同，并由连接数据库的角色拥有。Modary 把 pgx
`search_path` 固定为应用 schema；River 始终显式使用队列 schema。应用 schema 与
队列 schema 组成一一对应的耐久 Profile，Adapter 会持久化双向绑定并拒绝重新配对，
任何一个 schema 都不能被其他 Profile 共享，也不能通过交换 application/queue 角色
重新使用。两个 schema 使用相同结构的角色绑定标记；配置相同 schema 对的多个进程
可以并行启动。

两者放在同一个数据库中，是为了让受治理 Action 的控制状态写入和任务创建使用
内部同一个 `*sql.Tx` 原子提交。拆成两个数据库后，PostgreSQL 无法提供这个本地
事务保证。

## 业务数据

Modary 不要求业务数据存放在 PostgreSQL。应用可以通过自己定义的 Connector
访问 PostgreSQL、MySQL、数仓、对象存储或外部 API。Connector 不属于本地控制
数据库事务。

需要与任务创建原子提交的流程和治理状态应写入控制数据库。产品数据和外部副作用
应放在 Connector 后面，并使用稳定业务标识实现幂等。

## 事务内入队

`task.Service.Enqueue` 只能在受治理 Action 的事务上下文中调用。在事务之外调用
会返回 `task.ErrTransactionRequired`。因此不会出现以下两种分裂状态：

- 控制状态已经提交，但任务没有创建；
- 控制状态已经回滚，但队列中留下了任务。

公开 API 不暴露 River、PostgreSQL connection 或 transaction 类型。PostgreSQL
Adapter 在内部识别自己的上下文事务，并通过 River `InsertTx` 完成入队。

## 投递语义

任务采用 at-least-once 投递。进程崩溃、超时、租约丢失或完成状态持久化失败后，
Handler 都可能再次执行。框架提供 job ID、任务 kind、queue、当前 attempt、最大
attempt 和防御性复制后的 payload，但不承诺外部副作用 exactly-once。

应用应选择 Run ID 一类稳定业务标识。执行外部副作用前先读取当前产品状态；成功
后通过受治理 Action 写入终态；重复投递读取到终态时直接返回。外部系统支持
idempotency key 时，每次尝试都传入相同的稳定标识。

River 会持久化 Handler 返回的错误文本。Handler 应返回稳定、可展示且不含秘密的
错误，不要包装数据库 URL、密码、token、请求正文或第三方响应正文。详细依赖错误
应进入单独受保护的可观测系统。

## 进程拓扑和失败边界

API 进程可以只入队而不启动 Runner；Worker 进程可以为指定 queue 启动一个或多个
不可变 Runner。多个进程可安全消费同一 queue，任务租约和协调由 PostgreSQL 中的
River 负责。`MaxWorkers` 是单个 Runner 的并发上限，不是全局上限。

原子保证止于控制数据库。调用外部 API 或写入另一个数据库时，应通过幂等、对账或
产品自己的 saga 处理部分成功，不能把本地 PostgreSQL 事务描述为分布式事务。
