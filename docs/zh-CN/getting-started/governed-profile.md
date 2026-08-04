# Governed Profile 教程

> [English](../../getting-started/governed-profile.md)

Governed Profile 用于需要授权 Preview、精确执行计划、幂等重试、详细审计，以及
与业务状态一起提交耐久任务的高影响操作。

## 创建与配置

```bash
export MODARY_STARTER_REPLACE="$(pwd)"
go run ./cmd/modary new ../policy-control \
  --profile governed \
  --module example.com/acme/policy-control \
  --name "Policy Control"
cd ../policy-control
go mod tidy
```

River 不需要独立数据库，但应用与队列 schema 必须不同，并由当前数据库角色拥有：

Starter 根据项目 ID 派生带角色前缀的默认名称，使 application、queue 与 test
命名空间彼此分离，并避开 PostgreSQL 保留 schema；名称超过 PostgreSQL 或 River
上限时会保留可读前缀并加入确定性哈希片段。

```bash
export DATABASE_URL='postgres://user:password@127.0.0.1:5432/app?sslmode=disable'
export MODARY_APPLICATION_SCHEMA=modary_app_policy_control
export MODARY_QUEUE_SCHEMA=modary_queue_policy_control
export MODARY_OPERATOR_USERNAME=operator
export MODARY_OPERATOR_PASSWORD='development-password'
export MODARY_OPERATOR_TOKEN='development-bearer-token-000000000001'
export MODARY_ALLOW_INSECURE_COOKIE=true
```

## Preview 与 Execute

```bash
printf '%s' "$MODARY_OPERATOR_TOKEN" > token
chmod 600 token
printf '%s\n' '{"value":25,"expected_version":0}' > input.json

go run ./cmd/policy-control action run limits.set \
  --token-file token --input input.json --preview --request-id preview-1
```

Preview 会读取状态、授权意图和影响，并返回 `plan_hash`、当前/目标摘要、受影响资源
与过期时间，不修改状态也不入队。

```bash
go run ./cmd/policy-control action run limits.set \
  --token-file token --input input.json \
  --plan 'sha256:replace-with-preview-hash' \
  --idempotency-key workspace-default-limit-v1 \
  --request-id execute-1
```

Execute 在一个框架拥有的 PostgreSQL 事务中重新授权并提交状态、幂等结果、
required audit 和 River task。使用相同幂等键重试会返回已存结果，不重复写入。

## 运行 API 与 Worker

```bash
go run ./cmd/policy-control serve --listen 127.0.0.1:8080
go run ./cmd/policy-control-worker
```

Worker 通过公共 `task.Runner` 消费 `limits.changed`。交付语义是 at least once，
产品副作用必须使用稳定业务标识实现幂等。

## 验证

```bash
DATABASE_URL="$DATABASE_URL" go test ./...
go vet ./...
go build ./cmd/policy-control ./cmd/policy-control-worker
```

集成测试覆盖强制 Preview、RBAC default deny、Execute、幂等 replay、SQL Audit、
重启恢复和重启后 task 消费。

普通列表与编辑表单不要为了形式一致而进入 Preview。只有影响、自动化、审计或重试
要求值得额外成本时，才对具体操作采用 Governed。
