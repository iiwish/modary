# 可观测性

Modary 把必需的进程诊断和可选遥测分开。生成进程通过 `log/slog` 输出有界 JSON
日志。Admin 创建时选择 `--with otel`，才会引入独立版本的 `components/otel`
模块和 OTLP/HTTP trace、metric；未选择的项目不会依赖 OpenTelemetry SDK 或
exporter。

```bash
export MODARY_OTEL_ENDPOINT='https://collector.example.com:4318'
export MODARY_OTEL_ENVIRONMENT='production'
export MODARY_OTEL_HEADERS_JSON='{"Authorization":"Bearer ..."}'
```

只有 HTTP endpoint 才允许显式设置 `MODARY_OTEL_INSECURE=true`，并且只应用于
隔离的本地环境。endpoint 不允许包含 path、query、fragment 或 userinfo。Header、
服务标识、环境名、导出周期、超时和 readiness 超时都在启动前校验并限制大小。

HTTP 遥测只使用预检通过的 method、route template、status class、duration 和
active request。数据库与任务遥测只使用封闭的 operation 枚举。raw path、query、
SQL、payload、credential、actor ID 和 scope ID 都不会成为 metric label 或 span
attribute；exporter header 只用于请求 Collector。

Trace 与 metric exporter 在一次故障期间只写一条结构化
`telemetry.export.failed` 状态转换，恢复后写一条
`telemetry.export.recovered`。诊断只包含封闭的 `traces` 或 `metrics` signal，
不会记录 Collector 响应文本或 exporter header；返回给 OpenTelemetry SDK 的
错误也是不含秘密的稳定哨兵错误。

组件独立持有 trace provider、meter provider、reader 和 exporter，不修改全局
OpenTelemetry provider。应用关闭时在统一超时内 flush 并 shutdown。生成的遥测
Admin 会把 Collector 连通性加入 `/readyz`，但 `/livez` 始终只反映本地进程状态。

运维时应使用 TLS、限制 Collector 网络、轮换凭据、设置采样与保留策略，并通过
故障演练确认 Collector 中断、遥测导出失败和进程关闭都是有界的。日志、trace 和
metric 中不得出现请求体、token、authorization code、数据库 URL、raw SQL 或
任务 payload。
