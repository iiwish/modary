CREATE TABLE IF NOT EXISTS modary_audit_log (
    audit_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    actor_id TEXT,
    actor_type TEXT,
    channel TEXT,
    action_id TEXT NOT NULL,
    workspace_id TEXT,
    input_hash TEXT,
    plan_hash TEXT,
    decision TEXT NOT NULL,
    result_summary TEXT,
    error_code TEXT,
    reason TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_modary_audit_workspace_time
    ON modary_audit_log(workspace_id, finished_at DESC);

CREATE INDEX IF NOT EXISTS idx_modary_audit_action
    ON modary_audit_log(action_id, decision);
