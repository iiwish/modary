# 运行耐久后台任务

> [English](../../how-to/run-background-tasks.md)

该路径需要 Governed PostgreSQL 组件。API 和 Admin Profile 默认没有
`task.Service`。River 是内部实现，应用只导入公共 `task` 包。

## 1. 解析 Task Service

```go
tasks := application.Tasks()
if tasks == nil {
    return errors.New("durable task service is unavailable")
}
```

Module HandlerFactory 也可以在声明 `module.CapabilityTasks` 后解析同一服务。

## 2. 在 Governed Action 中入队

在 `Handler.Execute` 中调用 `Enqueue`。Runtime 已建立受治理事务，因此状态写入与
job insert 同时提交或回滚。

```go
_, err := tasks.Enqueue(ctx, task.Request{
    Kind:        "report.generate",
    Queue:       "reports",
    Payload:     json.RawMessage(`{"report_id":"report-42"}`),
    UniqueKey:   "report-42",
    MaxAttempts: 5,
})
```

Kind 和 Queue 使用稳定小写标识。Payload 必须是一个有效 JSON 值且不超过 1 MiB。
`UniqueKey` 用于抑制等价活动任务，但不能替代 Handler 自身幂等。

## 3. 实现幂等 Handler

严格解码 payload，按稳定业务 ID 查询状态；已经完成时返回 nil。调用下游系统时尽量
传递同一 ID 作为幂等键。返回错误会触发重试，错误文本可能被持久化，所以不要包装
连接串、凭据或原始响应。

## 4. 启动不可变 Runner

```go
runner, err := tasks.NewRunner(handler, task.RunnerOptions{
    Queues:          []task.Queue{{Name: "reports", MaxWorkers: 8}},
    JobTimeout:      10 * time.Minute,
    SoftStopTimeout: 30 * time.Second,
    RetryDelays:     []time.Duration{time.Second, 10 * time.Second, time.Minute},
})
if err != nil { return err }
if err := runner.Start(ctx); err != nil { return err }
```

需要修改队列或并发时创建新 Runner，不要在启动后修改配置。关闭时使用有界 context
调用 `Stop`。

## 5. 使用公共契约查看任务

运维界面使用 `task.Inspector`，不读取 River 表，也不使用 River 状态名。单页最多
100 条，可使用排他性的 `BeforeID` 游标，以及可选的队列和状态过滤。公共状态仅包含
`queued`、`pending`、`scheduled`、`running`、`retrying`、`succeeded`、
`failed` 和 `cancelled`；具体队列组件负责映射内部生命周期。

Task Summary 不暴露 payload 和后端持久化错误文本。Inspection 只读，不授予入队、
重试、取消或队列管理权限。

## 6. 测试原子边界

使用真实 PostgreSQL 覆盖：

1. 写业务状态并入队后返回错误，断言二者都不存在；
2. 再次执行并返回 nil，断言二者都存在；
3. 启动 Runner 并观察消费；
4. 覆盖重复交付、取消、终止重试、重启恢复与多 worker 竞争。

生产拓扑和数据库边界参见英文[部署](../../operations/deployment.md)与
[持久化与耐久任务](../../concepts/persistence-and-tasks.md)。
