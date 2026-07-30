CREATE TABLE IF NOT EXISTS modary_action_plan (
    plan_hash TEXT PRIMARY KEY,
    action_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    plan_json BLOB NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_modary_action_plan_expiry
    ON modary_action_plan(expires_at);

CREATE TABLE IF NOT EXISTS modary_action_idempotency (
    workspace_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    action_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'completed')),
    result_json BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, actor_id, action_id, idempotency_key)
);
