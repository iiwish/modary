CREATE TABLE modary_audit_event (
    event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id TEXT NOT NULL CHECK (length(request_id) BETWEEN 1 AND 128),
    actor_id TEXT NOT NULL DEFAULT '' CHECK (length(actor_id) <= 256),
    actor_type TEXT NOT NULL DEFAULT '' CHECK (length(actor_type) <= 64),
    channel TEXT NOT NULL DEFAULT '' CHECK (length(channel) <= 64),
    action_id TEXT NOT NULL CHECK (length(action_id) BETWEEN 1 AND 127),
    action_version TEXT NOT NULL DEFAULT '' CHECK (length(action_version) <= 128),
    contract_hash TEXT NOT NULL DEFAULT '' CHECK (length(contract_hash) IN (0, 71)),
    scope_kind TEXT NOT NULL DEFAULT '' CHECK (length(scope_kind) <= 64),
    scope_id TEXT NOT NULL DEFAULT '' CHECK (length(scope_id) <= 256),
    input_hash TEXT NOT NULL DEFAULT '' CHECK (length(input_hash) IN (0, 71)),
    plan_hash TEXT NOT NULL DEFAULT '' CHECK (length(plan_hash) IN (0, 71)),
    decision TEXT NOT NULL CHECK (decision IN ('allowed', 'denied', 'rejected', 'failed', 'idempotent_replay', 'previewed')),
    audit_level TEXT NOT NULL CHECK (audit_level IN ('metadata', 'detailed')),
    result_summary TEXT NOT NULL DEFAULT '' CHECK (length(result_summary) <= 512),
    impact_rows BIGINT CHECK (impact_rows IS NULL OR impact_rows >= 0),
    impact_resources_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(impact_resources_json) = 'array' AND jsonb_array_length(impact_resources_json) <= 32),
    result_references_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(result_references_json) = 'array' AND jsonb_array_length(result_references_json) <= 32),
    error_code TEXT NOT NULL DEFAULT '' CHECK (length(error_code) <= 64),
    error_kind TEXT NOT NULL DEFAULT '' CHECK (length(error_kind) <= 64),
    reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 2048),
    started_at TEXT NOT NULL CHECK (length(started_at) BETWEEN 20 AND 30),
    finished_at TEXT NOT NULL CHECK (length(finished_at) BETWEEN 20 AND 30)
);

CREATE INDEX modary_audit_event_scope ON modary_audit_event(scope_kind, scope_id, event_id DESC);
CREATE INDEX modary_audit_event_request ON modary_audit_event(request_id);
CREATE INDEX modary_audit_event_action ON modary_audit_event(action_id, event_id DESC);
