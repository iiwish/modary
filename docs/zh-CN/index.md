# Modary 中文文档

Modary 是面向业务系统和中后台的轻量、组件化 Go 后端框架。Core 只负责
Module 组合、Capability、生命周期和显式 HTTP 组合，不强制数据库、任务队列、
权限体系或前端。需要时再通过组件和 Profile 选择 PostgreSQL、登录、RBAC、React
后台、受治理 Action、审计与 River。

## 推荐上手路径

1. 先阅读[如何选择 Profile](getting-started/choose-profile.md)。
2. 用[快速上手](getting-started/quickstart.md)创建一个无数据库 API。
3. 根据产品需要完成 [Admin Profile 教程](getting-started/admin-profile.md)或
   [Governed Profile 教程](getting-started/governed-profile.md)。
4. 生产身份接入参考 [OIDC Admin 教程](getting-started/oidc-admin.md)。
5. 阅读[创建第一个应用](getting-started/first-application.md)，理解框架与产品边界。

## 核心主题

- [持久化与耐久任务](concepts/persistence-and-tasks.md)
- [运行耐久后台任务](how-to/run-background-tasks.md)
- [部署](operations/deployment.md)
- [可观测性](operations/observability.md)
- [完整 F0 框架契约](../framework-f0.md)
- [支持矩阵](../reference/support-matrix.md)
- [已知限制](../f0-known-limitations.md)
- [Rulary 基于 Modary 开发指南](../guides/rulary-bootstrap.md)

中文教程覆盖主要选择和实践路径。公共 API、部署与安全边界的完整规范以当前英文
契约为准。技术标识、包名、环境变量、命令和协议名保持原文。

- [English documentation](../index.md)
