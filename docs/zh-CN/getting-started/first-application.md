# 创建第一个独立应用

> [English](../../getting-started/first-application.md)

使用 `modary new` 创建独立 Go Module，不要复制框架内部示例。生成结果只依赖
Modary 公共包，不包含框架实现副本。

## 创建项目

```bash
export MODARY_CHECKOUT=/absolute/path/to/modary
export MODARY_STARTER_REPLACE="$MODARY_CHECKOUT"
cd "$MODARY_CHECKOUT"
go run ./cmd/modary new /absolute/path/to/billing-api \
  --profile api \
  --module company.example/platform/billing-api \
  --name "Billing API"
```

目标目录名必须是小写项目 ID，父目录必须存在，符号链接和非空目录会被拒绝。Go
Module Path 不能包含名为 `vendor` 的路径段，因为 Go 将其保留给 vendored import。

## 验证独立边界

```bash
cd /absolute/path/to/billing-api
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go build ./...
```

发布应用前删除本地 `replace`，并在 Modary 对应版本实际发布后固定准确标签：

```bash
go mod edit -dropreplace github.com/iiwish/modary
go get github.com/iiwish/modary@v0.3.0-alpha.1
go mod tidy
```

在该版本尚未发布时不要伪造或依赖这个远程标签。

## 添加业务 Module

例如创建 `internal/invoices`，由它负责稳定 Module ID、迁移、Repository、领域服务、
HTTP 路由或受治理 Action，以及相关测试。然后在
`internal/app/application.go` 中显式注册。

业务表名、状态、角色、导航、校验文案和部署配置都属于应用。不要把产品概念放入
Modary，也不要增加自动扫描器作为第二套组合模型。

## 选择升级方式

Starter 只创建一次，不会修补产品源码。升级 Modary 时固定新版本，先编译，再根据
升级指南手工适配公共接口和组合入口，检查依赖图并运行外部验收。这样框架不会在
不理解产品意图的情况下改写应用。
