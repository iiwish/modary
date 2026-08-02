# Modary 快速上手

> [English](../../getting-started/quickstart.md)

本教程运行公开的 Counter Console 示例，预览一个受治理的 Action，并说明框架与
应用各自负责什么。该示例本身是独立 Go Module，也是本项目复制到仓库外和远程
版本消费时使用的验收应用。

## 1. 获取源码

```bash
git clone --branch v0.1.0-alpha.3 https://github.com/iiwish/modary.git
cd modary
make bootstrap
```

`make bootstrap` 会在禁用 Go work file 的条件下下载框架与示例依赖。需要
Go 1.26 或更高版本，不需要 Node.js。

如果已经有 Modary 源码目录，请从仓库根目录运行同一个 `make bootstrap` 命令。

## 2. 启动 PostgreSQL

官方持久化 Profile 需要 PostgreSQL 17。启动一个隔离的本地数据库，并让应用与
集成测试使用同一个连接地址：

```bash
docker run --name modary-counter-postgres \
  -e POSTGRES_DB=modary_counter \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  -d postgres:17-alpine
docker exec modary-counter-postgres pg_isready -U postgres -d modary_counter
export MODARY_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/modary_counter?sslmode=disable'
export MODARY_TEST_DATABASE_URL="$MODARY_DATABASE_URL"
```

示例会创建并拥有保存应用控制状态的 `counter_app` schema，以及供 River 使用的
`counter_queue` schema。生产环境应使用受限角色、TLS、托管凭据、备份和监控，
详见[部署](../../operations/deployment.md)。

## 3. 验证公开示例

```bash
cd examples/counter
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go run ./tools/modary check
GOWORK=off go test ./...
```

预期结果是所有命令成功退出，已生成文件没有漂移。`verify` 只检查纯
`Definition`，不会打开数据库、执行迁移、创建 Handler 或启动 Module。

## 4. 构建并检查应用命令

```bash
GOWORK=off go run ./tools/modary build
GOWORK=off ./dist/counter-console version
GOWORK=off ./dist/counter-console help
```

`version` 输出 `counter-console 0.1.0`。`help` 和 `version` 都是纯路径，
不会连接 PostgreSQL。

## 5. 预览受治理的 Action

创建本地教程输入和受保护的 token 文件：

```bash
printf '%s' 'counter-primary-bearer-token-000000000001' > /tmp/modary-counter-token
chmod 0600 /tmp/modary-counter-token
printf '%s\n' '{"amount":1,"expected_version":0}' > /tmp/modary-counter-input.json
```

预览 Action：

```bash
GOWORK=off ./dist/counter-console action run counter.increment \
  --token-file /tmp/modary-counter-token \
  --input /tmp/modary-counter-input.json \
  --preview
```

返回的 JSON 包含 Counter 的当前状态、下一状态和绑定后的 `plan_hash`。Preview
会认证调用者、校验输入、授权意图、读取状态并绑定预期效果，但不会执行写入。

使用完毕后删除教程文件：

```bash
rm -f /tmp/modary-counter-token /tmp/modary-counter-input.json
```

示例中的 token 和密码是公开的本地演示凭据，不得用于其他应用或部署环境。

## 6. 理解组合入口

[组合入口](../../../examples/counter/internal/project/project.go) 创建官方 PostgreSQL、
Local Identity、RBAC 和 SQL Audit Adapter，然后注册应用自己的 Module。
[应用命令](../../../examples/counter/cmd/counter-console/main.go)与
[项目工具](../../../examples/counter/tools/modary/main.go)使用同一个纯
`Definition` provider。

[Counter Module](../../../examples/counter/modules/counter/module.go)负责自己的迁移、
Action descriptor、Handler factory、Preview plan、执行逻辑和公开冲突错误。
框架负责生命周期、Capability 校验、授权、计划绑定、幂等、事务边界和强制审计。

## 7. 继续创建应用

继续阅读[创建第一个独立应用](first-application.md)。下一篇会把示例复制到 Modary
仓库之外，删除开发期 `replace`，验证公开版本解析，并完成第一次生成契约变更。

需要深入理解时，请查看英文的[项目结构](../../getting-started/project-layout.md)、
[受治理 Action](../../concepts/governed-actions.md)和
[故障排查](../../how-to/troubleshooting.md)。

完成教程后停止本地数据库：

```bash
docker rm -f modary-counter-postgres
```
