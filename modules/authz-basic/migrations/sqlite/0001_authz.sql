CREATE TABLE IF NOT EXISTS modary_role_binding (
    user_id TEXT NOT NULL REFERENCES modary_user(user_id),
    workspace_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    PRIMARY KEY (user_id, workspace_id, role_id)
);

CREATE TABLE IF NOT EXISTS modary_role_permission (
    role_id TEXT NOT NULL,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE IF NOT EXISTS modary_agent_grant (
    grant_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    delegated_by TEXT NOT NULL REFERENCES modary_user(user_id),
    workspace_id TEXT NOT NULL,
    actions_json BLOB NOT NULL,
    max_rows INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    grant_version TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);
