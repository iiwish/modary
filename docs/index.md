# Modary Documentation

Modary is a Go-first framework for applications that need explicit module
composition and one governed path for business Actions across CLI, HTTP, MCP,
and consumer-owned surfaces. Use this index by task or audience. The detailed
F0 contract is authoritative when a guide and a deep contract appear to differ.

## 简体中文

- [中文文档入口](zh-CN/index.md)
- [快速上手](zh-CN/getting-started/quickstart.md)
- [创建第一个独立应用](zh-CN/getting-started/first-application.md)
- [持久化与耐久任务](zh-CN/concepts/persistence-and-tasks.md)
- [运行耐久后台任务](zh-CN/how-to/run-background-tasks.md)

## Start Building

- [Installation and version pinning](getting-started/installation.md)
- [Quickstart with the executable consumer](getting-started/quickstart.md)
- [Create your first independent application](getting-started/first-application.md)
- [Canonical consumer project layout](getting-started/project-layout.md)

## Understand The Model

- [Framework and consumer ownership](concepts/consumer-boundary.md)
- [Modules, capabilities, and lifecycle](concepts/modules-and-capabilities.md)
- [Governed Action lifecycle](concepts/governed-actions.md)
- [Persistence and durable task model](concepts/persistence-and-tasks.md)

## Complete Common Tasks

- [Add a consumer module](how-to/add-module.md)
- [Expose one Action through supported surfaces](how-to/expose-action.md)
- [Enqueue and run durable background tasks](how-to/run-background-tasks.md)
- [Test a Modary application](how-to/test-application.md)
- [Troubleshoot common first-run failures](how-to/troubleshooting.md)

## Look Up Contracts

- [Public package map](reference/packages.md)
- [Support matrix](reference/support-matrix.md)
- [Project manifest and generated files](reference/project-manifest.md)
- [Complete F0 framework contract](framework-f0.md)
- [Known limitations](f0-known-limitations.md)
- [F0 acceptance report](f0-acceptance-report.md)

## Operate And Secure

- [Deployment profile and production checklist](operations/deployment.md)
- [Security boundaries and secret handling](operations/security.md)
- [PostgreSQL backup and restore](operations/postgresql-backup-restore.md)

## Release And Upgrade

- [Versioning and compatibility](releases/versioning.md)
- [Release process](releases/release-process.md)
- [Consumer upgrade guide](releases/upgrade-guide.md)
- [Changelog](../CHANGELOG.md)
- [Security policy](../SECURITY.md)
- [Contribution guide](../CONTRIBUTING.md)

## Architecture Decisions

- [ADR-001: explicit composition and capability lifecycle](adr/ADR-001-explicit-composition-and-capability-lifecycle.md)
- [ADR-002: governed Action transaction](adr/ADR-002-governed-action-transaction.md)
- [ADR-003: PostgreSQL and module migrations](adr/ADR-003-postgresql-and-module-migrations.md)
- [ADR-004: consumer-owned surfaces](adr/ADR-004-consumer-owned-surfaces.md)

## Choose The Right Depth

Application developers usually need the quickstart, concepts, how-to guides,
and package reference. Operators should read the support matrix and every
operations guide. Framework contributors should also read the F0 contract,
ADRs, contribution guide, and release policy. Security reviewers should treat
the security and known-limitations documents as required, not optional notes.
