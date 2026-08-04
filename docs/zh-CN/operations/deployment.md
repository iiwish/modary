# 部署

Modary 的部署拓扑由显式组件图决定。框架提供可移植的进程契约和消费者拥有的参考
源码，但不代管 PostgreSQL、TLS、容器调度、Secret、身份提供方或 Collector。

## 容器产物

三个 Profile 都会生成多阶段 `Dockerfile` 和 `.dockerignore`；Admin 与 Governed
还会生成 PostgreSQL `compose.yaml`。最终镜像只包含静态 Go 应用与 CA 根证书：

- 以数字用户和组 `65532:65532` 运行；
- 支持只读根文件系统、`/tmp` tmpfs、删除全部 Linux Capability；
- 不包含 Go、Node.js、源码、前端源码、VCS 信息或包缓存；
- 记录 OCI version、revision、created label，并把相同构建身份写入结构化日志；
- 通过 `TARGETOS` 与 `TARGETARCH` 构建 Linux amd64 或 arm64 镜像。

生成内容是可审查的起点。产品仓库应固定基础镜像 digest，执行组织选定的镜像扫描，
并在发布记录中保存最终镜像 digest。

```bash
go mod tidy
docker build \
  --build-arg VERSION=v1.4.0 \
  --build-arg REVISION="$(git rev-parse HEAD)" \
  --build-arg CREATED="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t registry.example.com/acme/service:v1.4.0 .
```

`CREATED` 必须是规范 RFC3339。构建阶段显式使用 `GOTOOLCHAIN=local`、
`GOWORK=off` 和 `-mod=readonly`，因此 `go.mod` 与 `go.sum` 必须先处于已审核状态。

## 探针与流量

- `GET` 或 `HEAD /livez` 只表示本地进程仍能推进，不访问外部依赖；
- `GET` 或 `HEAD /readyz` 反映启动、已选依赖检查和排空状态；
- 探针拒绝 query 与 body，响应是小型、`no-store` 的 JSON；
- PostgreSQL 或 Collector 故障可以让实例保持 live 但变为 unready。

编排平台应在 unready 时摘除流量，而不是因为依赖短暂故障重启仍然 live 的进程。
不要增加第二套含义不清的健康端点。
生成服务器同时限制 header 字节数、header/read/write/idle/shutdown 超时。上传或
流式产品路由需要有意识地调整边界，不应在全局关闭这些限制。

## 独立迁移

数据库 Profile 提供一次性 `migrate` 命令。先用候选镜像执行迁移，成功后再启动新
实例：

```bash
./application migrate
./application serve
```

迁移进程只加载 `DATABASE_URL` 和已选 schema 名称，不需要 Admin 密码、OIDC
Client Secret、Operator 凭据或 OTLP Header。运行时身份初始化和外部 Provider
启动只发生在 Serve 或 Worker 进程。`MODARY_MIGRATE_ON_START=true` 仅适合明确的
本地单实例流程。已提交的迁移是前向且不可编辑的，后续启动失败不会自动回滚迁移。

## Profile 拓扑

API 只有一个 HTTP 进程，无数据库与 Worker。Admin 使用一个或多个 Go API/UI
实例和同一个 PostgreSQL 应用 schema；React Bundle 已嵌入二进制。`--with tasks`
增加 River，`--with audit` 增加审计读取，`--with otel` 连接外部 OTLP/HTTP
Collector，`--with oidc` 替换本地密码登录。

本 Alpha 的 OIDC 待完成跳转保存在发起进程内。登录开始到 Callback 之间使用单
登录实例或入口会话亲和性；建立后的应用 Session 保存在 PostgreSQL，不需要该亲和
策略。

Governed 使用独立 API 与 Worker 进程，连接一个物理 PostgreSQL 数据库中的应用
schema 和 River schema。Worker 停止时 API 仍可入队；如果改用独立队列数据库，
就不能继续声明业务写入与任务入队位于同一个事务。

## 排空、Secret 与验收

SIGINT 或 SIGTERM 到达后，进程先关闭 readiness 与新业务请求，再等待已接收请求，
关闭 HTTP，按依赖逆序释放 Module。平台 termination grace period 必须大于应用、
Worker 和 Exporter 的关闭预算；业务 Callback 与 Task Handler 必须响应 Context。

通过部署 Secret 机制传入数据库、OIDC、密码和 OTLP 凭据，禁止写入源码、镜像层、
Label、命令参数或日志。TLS、可信 Proxy/Host、Origin、安全 Header、WAF、限流、
网络出口、数据库角色、备份与 Collector 访问控制由产品和平台负责。

发布前至少验证：非 Root、只读文件系统、无 Node.js/源码/Secret、OCI Label、独立
迁移、两个探针、PostgreSQL 与 Collector 中断、活动请求 SIGTERM 排空、超时退出、
安全 Cookie、OIDC Callback、备份恢复，以及 River Lag/Retry/Discard 监控。
