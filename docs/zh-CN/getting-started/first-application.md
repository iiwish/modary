# 创建第一个独立应用

> [English](../../getting-started/first-application.md)

本教程把已发布源码中的 Counter 示例变成一个普通应用目录。新应用通过公开的
Go Module 标签解析 Modary，不依赖 Modary 源码目录。开始前请先完成
[快速上手](quickstart.md)。

## 1. 复制示例

在已发布 Modary 源码目录的上一级执行：

```bash
cp -R modary/examples/counter my-counter
cd my-counter
```

复制后的目录已经是独立 Go Module，包含自己的组合入口、项目命令、应用命令、
业务 Module、迁移、生成契约、测试和静态 UI。

## 2. 选择公开框架版本

删除源码检出态使用的本地绑定，并固定到准确的 Alpha 版本：

```bash
GOWORK=off go mod edit -dropreplace=github.com/iiwish/modary
GOWORK=off go mod edit -require=github.com/iiwish/modary@v0.1.0-alpha.2
GOWORK=off go mod tidy
```

确认 Go 解析到了公开版本，并且没有使用 `replace`：

```bash
GOWORK=off go list -m -f '{{.Path}} {{.Version}} {{if .Replace}}replaced{{end}}' github.com/iiwish/modary
```

预期输出：

```text
github.com/iiwish/modary v0.1.0-alpha.2
```

在修改示例之前，记录这个已经能够远程解析的起点：

```bash
git init
git add .
git commit -m "Bootstrap Modary application"
```

## 3. 验证应用契约

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go test ./...
GOWORK=off go run ./tools/modary build
GOWORK=off ./dist/counter-console version
```

最后一个命令输出 `counter-console 0.1.0`。这些命令都不需要 Node.js，也不需要
Modary 源码目录。

## 4. 使用真实 Module 路径

发布应用前，把 `go.mod` 和应用内部 import 中的
`example.com/modary-counter-consumer` 替换为应用的正式 Module 路径。应使用编辑器
提供的 Go-aware rename，然后重新运行 `go mod tidy` 和上一节的完整命令。不要修改
以 `github.com/iiwish/modary` 开头的框架 import。

## 5. 完成第一次契约变更

打开 `modules/counter/module.go`，修改 `descriptor()` 内 Action 的 `Title` 或
`Description`。此时已提交的生成目录会变为过期状态：

```bash
GOWORK=off go run ./tools/modary generate --check
```

命令应失败并报告 generated drift。重新生成、检查差异并恢复全部绿色检查：

```bash
GOWORK=off go run ./tools/modary generate
git diff -- internal/generated
GOWORK=off go run ./tools/modary check
GOWORK=off go test ./...
```

这就是正常的契约开发流程：修改纯 `Definition` 元数据，生成可审查的产物，并把
源码和生成结果一起提交。

## 6. 替换示例业务

把英文[项目结构](../../getting-started/project-layout.md)作为稳定的仓库骨架，按照
[添加 Module](../../how-to/add-module.md)创建应用自己的 Module。只有当新 Module、
迁移、Action、组合注册、生成产物和测试一起通过后，才删除 Counter 代码。

开发期 `replace` 只适合 Modary 和应用同时在本地检出时使用，不得提交到应用的
发布分支。Module 解析、生成、Capability、迁移或平台检查失败时，请查看英文
[故障排查](../../how-to/troubleshooting.md)。
