# 持久化与耐久任务

> [English](../../concepts/persistence-and-tasks.md)

Modary Core 不需要数据库。持久化通过两套权限不同的 PostgreSQL 组件进入应用。

## 普通 PostgreSQL

`components/postgres` 提供 `database.Store` 和一个应用 schema，不安装 River、
Action plan、幂等或审计存储。Repository 通过 `Store.WithinTransaction` 完成普通
业务事务，Store 负责 begin、commit 与 rollback，不暴露原始连接。

组件会用 `application` 角色标记保留其物理 schema。Governed 组件可以继续把同一
schema 用作 application schema，并将它与唯一的 queue schema 配对；任何组件都
不能把 application schema 改作 queue schema，普通组件也会拒绝已标记为 queue
的 schema。

这是 Admin Profile 的默认路径，适合普通 CRUD、草稿、元数据和内部后台操作。

## Governed PostgreSQL 与 River

`components/governedpostgres` 提供受治理 Action 持久化、`database.Access` 和公共
`task.Service`。同一个物理 PostgreSQL 数据库包含两个不同 schema：

- application schema：Module 历史、产品控制表、plan、幂等、Identity、RBAC、audit；
- queue schema：River migration、job、协调状态与 Modary task-inspection 索引。

River 需要 schema 和表，不需要专用数据库。同一数据库使受治理状态写入与 job
insert 进入同一个内部事务。`database.Access` 不能自行开始事务；事务外调用
`task.Service.Enqueue` 会失败。schema 标识符仅使用小写 ASCII 字母、数字与下划线；
application schema 最长 63 字节，River queue schema 最长 46 字节，以确保添加前缀后的
PostgreSQL notification topic 仍是合法标识符。

Governed 组件拥有三组 queue-schema 索引，对齐公共 Inspector 的倒序 ID 游标以及
可选的 queue/state 过滤，为后台读取提供索引对齐路径，不再因缺少可用索引而被迫
全表扫描或无界排序。这些索引也会增加 River job 写入时正常的 B-tree 维护成本，
不应脱离组件单独重命名或删除。

Task 与 audit ID 在 Go 中保持有符号 64 位整数；Admin JSON 的 `id` 与
`next_before_id` 使用十进制字符串，避免浏览器显示或回传游标时发生 JavaScript
number 精度丢失。

## 业务数据库选择

F0 官方优先支持 PostgreSQL。Core 契约保持 provider-neutral，应用可以在自己的
Module 后实现其他数据库或服务，但当前没有官方 MySQL 或 SQLite conformance。
外部数据库无法加入 governed PostgreSQL 本地事务，需要幂等、对账或产品 saga。

## 交付语义

River 是 at least once。超时、lease 丢失、进程崩溃或完成状态保存失败都可能导致
重复执行。Handler 应使用稳定产品标识，并让外部副作用幂等。错误可能保存在 River
历史中，因此返回有界且不含密钥的错误，把基础设施详情写入受保护观测系统。

## 如何选择

- 无持久化：API Profile。
- 普通业务 CRUD：`postgresdb` 与 `database.Store`。
- 需要 Preview、审计、幂等以及原子 task insert 的变更：`governedpostgres`、
  `action.Runtime` 与 `task.Service`。

不要因为“需要异步函数”就选择 River；只有耐久、重试和事务化记录意图是产品要求
时才值得采用。
