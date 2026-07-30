# F0 Permission Matrix

All requests are scoped to `actor.workspace_id == request.workspace_id`.
Authorization runs before planning and again against planned impact before
execution.

## Roles

| Role | Create | Edit | Validate / Preview | Publish | Execute | Audit |
|---|---:|---:|---:|---:|---:|---:|
| `rulary_author` | Yes | Yes | Yes | No | No | No |
| `rulary_publisher` | Yes | Yes | Yes | Yes | No | No |
| `rulary_operator` | No | No | Yes | No | Yes | No |
| `rulary_auditor` | No | No | No | No | No | Yes |
| `workspace_admin` | Yes | Yes | Yes | Yes | Yes | Yes |

## Actions

| Action | Required permission | Channels | Preview policy | Idempotency |
|---|---|---|---|---|
| `rulary.ruleset.list` | `rulary.ruleset.preview` | HTTP, CLI | None | No |
| `rulary.ruleset.get` | `rulary.ruleset.preview` | HTTP, CLI | None | No |
| `rulary.ruleset.create` | `rulary.ruleset.create` | HTTP, CLI | None | Required |
| `rulary.ruleset.update_draft` | `rulary.ruleset.edit` | HTTP | None | Required |
| `rulary.ruleset.validate` | `rulary.ruleset.preview` | HTTP, CLI, MCP | None | Yes |
| `rulary.ruleset.preview` | `rulary.ruleset.preview` | HTTP, CLI, MCP | Optional | No |
| `rulary.ruleset.publish` | `rulary.ruleset.publish` | HTTP | Required | Required |
| `rulary.run.execute` | `rulary.run.execute` | HTTP, CLI, MCP | Required | Required |
| `rulary.run.get` | `rulary.run.execute` | HTTP, CLI | None | No |
| `audit.query` | `audit.read` | HTTP, CLI | None | No |

## Agent Grant

The F0 agent is delegated by `user_operator`, is limited to workspace
`ws_default`, and can call only `rulary.ruleset.validate`,
`rulary.ruleset.preview`, and `rulary.run.execute`. Planned impact is capped at
50 rows. Grant expiry, allowlist, delegated permissions, workspace, impact, and
plan binding are all enforced by the shared runtime path.
