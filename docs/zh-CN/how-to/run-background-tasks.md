# 运行耐久后台任务

本教程创建一个在事务内入队的任务，并启动 Worker Runner。PostgreSQL Adapter
内部使用 River，应用代码只导入公开 `task` 包。

## 1. 获取任务服务

组装后的应用提供公开服务：

```go
tasks := application.Tasks()
if tasks == nil {
    return errors.New("durable task service is unavailable")
}
```

Module 声明 `module.CapabilityTasks` 后，也可以在 Handler factory 中通过
`module.Resolve(services, module.Tasks())` 获取同一个服务。

## 2. 在受治理 Action 中入队

在 `Handler.Execute` 内调用 `Enqueue`。Runtime 已经打开受治理事务，因此控制表
写入和任务插入共享一次提交：

```go
receipt, err := tasks.Enqueue(ctx, task.Request{
    Kind:        "report.generate",
    Queue:       "reports",
    Payload:     json.RawMessage(`{"report_id":"report-42"}`),
    UniqueKey:   "report-42",
    MaxAttempts: 5,
})
if err != nil {
    return action.Result{}, fmt.Errorf("enqueue report: %w", err)
}
```

任务 kind 使用稳定的小写标识。Queue 名称最多 64 字节，可包含小写字母、数字、
下划线和连字符。Payload 必须是一个有效 JSON 值，最大 1 MiB。`UniqueKey` 可选，
用于抑制相同活动 kind/key 的重复插入；它最多 256 字节，不能包含控制字符或首尾
空白。

## 3. 实现幂等 Handler

```go
handler := task.HandlerFunc(func(ctx context.Context, job task.Job) error {
    var input struct {
        ReportID string `json:"report_id"`
    }
    if err := json.Unmarshal(job.Payload, &input); err != nil {
        return errors.New("report task payload is invalid")
    }

    // 先按 ReportID 读取产品状态；已完成时直接返回 nil。
    // 外部系统支持幂等键时传入同一个 ReportID。
    return generateReport(ctx, input.ReportID)
})
```

返回错误会让 River 按策略重试，直至任务成功或耗尽尝试次数。
`job.TerminalAttempt()` 表示当前是否为配置的最后一次尝试，可用于写入产品终态。
错误文本会被 River 保存，因此应返回稳定且不含秘密的错误，而不是包装 Connector
或基础设施错误。

## 4. 启动不可变 Runner

```go
runner, err := tasks.NewRunner(handler, task.RunnerOptions{
    Queues:          []task.Queue{{Name: "reports", MaxWorkers: 8}},
    JobTimeout:      10 * time.Minute,
    SoftStopTimeout: 30 * time.Second,
    RetryDelays:     []time.Duration{time.Second, 10 * time.Second, time.Minute},
})
if err != nil {
    return err
}
if err := runner.Start(ctx); err != nil {
    return err
}
```

需要调整 queue 或并发时创建新的 Runner，不要在启动后修改配置。空
`RetryDelays` 使用 River 默认策略；超过列表长度的尝试会复用最后一个延迟。进程
退出时用有截止时间的 context 调用 `Stop`，需要协调其他资源时再等待 `Stopped`。

## 5. 验证事务和恢复

集成测试使用真实 PostgreSQL，并至少覆盖：

1. 插入控制行并入队后返回错误，断言两者都不存在；
2. 再次执行并成功提交，断言控制行和 job 同时存在；
3. 启动 Runner，验证重复投递、取消、重试到终态、重启恢复和多 Worker 竞争。

数据库边界详见[持久化与耐久任务](../concepts/persistence-and-tasks.md)，生产拓扑详见
[Deployment](../../operations/deployment.md)。
