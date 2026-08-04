# Modary 快速上手

> [English](../../getting-started/quickstart.md)

本教程从当前源码创建无数据库 API Profile，这是理解 Modary 所有权边界的最短路径。

## 1. 准备框架

```bash
git clone https://github.com/iiwish/modary.git
cd modary
make bootstrap
export MODARY_STARTER_REPLACE="$(pwd)"
```

本地框架开发可通过 `MODARY_STARTER_REPLACE` 绑定当前检出。正式消费者应删除
`replace` 并准确固定到 `v0.3.0-alpha.1`。

## 2. 创建 API 项目

```bash
go run ./cmd/modary new ../inventory-api \
  --profile api \
  --module example.com/acme/inventory-api \
  --name "Inventory API"
```

命令返回目标目录、Profile 和排序后的文件列表。目标必须是新目录或空目录；再次
对同一目录执行会失败且不修改任何文件。

## 3. 检查显式组合

打开 `../inventory-api/internal/app/application.go`。Definition 只注册应用自己的
`ping` Module，HTTP 路由也是显式挂载，不存在源码扫描或隐藏数据库。

```bash
cd ../inventory-api
GOWORK=off go mod tidy
GOWORK=off go list -deps ./... | rg 'river|postgres|identitystore|sqlaudit'
```

最后一条命令应无输出，这证明未选择的组件不在依赖图中。

## 4. 测试与运行

```bash
GOWORK=off go test ./...
GOWORK=off go build ./cmd/inventory-api
go run ./cmd/inventory-api
```

另开终端验证：

```bash
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/ping
```

使用 `Ctrl-C` 停止。应用会排空 HTTP 请求并执行 Host 的 exactly-once shutdown。

## 5. 下一步

- 创建内部后台：阅读 [Admin Profile 教程](admin-profile.md)。
- 学习高影响命令：阅读 [Governed Profile 教程](governed-profile.md)。
- 添加业务 Module：阅读英文[添加 Module](../../how-to/add-module.md)。
