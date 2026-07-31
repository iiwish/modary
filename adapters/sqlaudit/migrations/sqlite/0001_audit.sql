CREATE TABLE modary_audit_event (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    actor_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT '',
    channel TEXT NOT NULL DEFAULT '',
    action_id TEXT NOT NULL,
    action_version TEXT NOT NULL DEFAULT '',
    contract_hash TEXT NOT NULL DEFAULT '',
    scope_kind TEXT NOT NULL DEFAULT '',
    scope_id TEXT NOT NULL DEFAULT '',
    input_hash TEXT NOT NULL DEFAULT '',
    plan_hash TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL,
    audit_level TEXT NOT NULL,
    result_summary TEXT NOT NULL DEFAULT '',
    impact_rows INTEGER,
    impact_resources_json TEXT NOT NULL DEFAULT '[]',
    result_references_json TEXT NOT NULL DEFAULT '[]',
    error_code TEXT NOT NULL DEFAULT '',
    error_kind TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL
);

CREATE INDEX modary_audit_event_scope
    ON modary_audit_event(scope_kind, scope_id, event_id DESC);

CREATE INDEX modary_audit_event_request
    ON modary_audit_event(request_id);

CREATE INDEX modary_audit_event_action
    ON modary_audit_event(action_id, event_id DESC);
