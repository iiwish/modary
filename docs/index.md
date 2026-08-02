# Modary Documentation

Modary is a lightweight component-oriented Go framework for business systems
and administrative backends. Start with the smallest Profile, keep the
generated composition explicit, and add only the components and consumer
Modules the product needs.

## Start Here

1. [Install or use the current source](getting-started/installation.md).
2. [Choose the API, Admin, or Governed Profile](getting-started/choose-profile.md).
3. [Create and run a project](getting-started/quickstart.md).
4. [Understand the generated layout](getting-started/project-layout.md).
5. [Add a consumer Module](how-to/add-module.md).

Profile tutorials:

- [API and first application](getting-started/first-application.md)
- [Admin Profile](getting-started/admin-profile.md)
- [Governed Profile](getting-started/governed-profile.md)

## 简体中文

- [中文文档入口](zh-CN/index.md)
- [选择 Profile](zh-CN/getting-started/choose-profile.md)
- [快速上手](zh-CN/getting-started/quickstart.md)
- [创建第一个独立应用](zh-CN/getting-started/first-application.md)
- [Admin Profile 教程](zh-CN/getting-started/admin-profile.md)
- [Governed Profile 教程](zh-CN/getting-started/governed-profile.md)
- [持久化与任务](zh-CN/concepts/persistence-and-tasks.md)
- [运行耐久后台任务](zh-CN/how-to/run-background-tasks.md)

## Concepts

- [Framework and consumer ownership](concepts/consumer-boundary.md)
- [Components and Profiles](concepts/components-and-profiles.md)
- [Modules, capabilities, and lifecycle](concepts/modules-and-capabilities.md)
- [Ordinary and governed persistence](concepts/persistence-and-tasks.md)
- [Governed Action lifecycle](concepts/governed-actions.md)

## Common Work

- [Add a consumer Module](how-to/add-module.md)
- [Expose a governed Action](how-to/expose-action.md)
- [Run durable background work](how-to/run-background-tasks.md)
- [Test an application](how-to/test-application.md)
- [Troubleshoot startup and composition](how-to/troubleshooting.md)
- [Plan Rulary adoption](guides/rulary-bootstrap.md)

## Operations And Security

- [Deployment](operations/deployment.md)
- [Security boundaries](operations/security.md)
- [PostgreSQL backup and restore](operations/postgresql-backup-restore.md)
- [Known limitations](f0-known-limitations.md)

## Reference

- [Public package map](reference/packages.md)
- [Support matrix](reference/support-matrix.md)
- [Optional project manifest tooling](reference/project-manifest.md)
- [Complete F0 contract](framework-f0.md)
- [Current F0 acceptance report](f0-acceptance-report.md)
- [Versioning](releases/versioning.md)
- [Upgrade from Alpha 3](releases/upgrade-guide.md)
- [Release process](releases/release-process.md)

## Architecture Decisions

- [ADR-001: explicit composition and lifecycle](adr/ADR-001-explicit-composition-and-capability-lifecycle.md)
- [ADR-002: governed transaction](adr/ADR-002-governed-action-transaction.md)
- [ADR-003: PostgreSQL and migrations](adr/ADR-003-postgresql-and-module-migrations.md)
- [ADR-004: consumer-owned surfaces](adr/ADR-004-consumer-owned-surfaces.md)
- [ADR-005: create-only Profiles and optional Admin UI](adr/ADR-005-create-only-profiles.md)

## Project Policy

- [Changelog](../CHANGELOG.md)
- [Contributing](../CONTRIBUTING.md)
- [Security policy](../SECURITY.md)
