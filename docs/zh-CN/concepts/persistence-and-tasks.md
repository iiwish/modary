# 持久化与耐久任务

> [English](../../concepts/persistence-and-tasks.md)

Modary Core 不需要数据库。持久化通过两套权限不同的 PostgreSQL 组件进入应用。

## 普通 PostgreSQL

`adapters/postgresdb` 提供 `database.Store` 和一个应用 schema，不安装 River、
Action plan、幂等或审计存储。Repository 通过 `Store.WithinTransaction` 完成普通
业务事务，Store 负责 begin、commit 与 rollback，不暴露原始连接。

这是 Admin Profile 的默认路径，适合普通 CRUD、草稿、元数据和内部后台操作。

## Governed PostgreSQL 与 River

`adapters/postgres` 提供受治理 Action 持久化、`database.Access` 和公共
`task.Service`。同一个物理 PostgreSQL 数据库包含两个不同 schema：

- application schema：Module 历史、产品控制表、plan、幂等、Identity、RBAC、audit；
- queue schema：River migration、job 与协调状态。

River 需要 schema 和表，不需要专用数据库。同一数据库使受治理状态写入与 job
insert 进入同一个内部事务。`database.Access` 不能自行开始事务；事务外调用
`task.Service.Enqueue` 会失败。

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
- 需要 Preview、审计、幂等以及原子 task insert 的变更：`postgres`、
  `action.Runtime` 与 `task.Service`。

不要因为“需要异步函数”就选择 River；只有耐久、重试和事务化记录意图是产品要求
时才值得采用。
