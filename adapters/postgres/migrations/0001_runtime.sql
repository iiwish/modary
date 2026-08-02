CREATE TABLE modary_action_plan (
    plan_hash TEXT PRIMARY KEY CHECK (length(plan_hash) = 71),
    action_id TEXT NOT NULL CHECK (length(action_id) BETWEEN 1 AND 127),
    action_version TEXT NOT NULL CHECK (length(action_version) BETWEEN 1 AND 128),
    contract_hash TEXT NOT NULL CHECK (length(contract_hash) = 71),
    actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 256),
    actor_type TEXT NOT NULL CHECK (length(actor_type) BETWEEN 1 AND 64),
    channel TEXT NOT NULL CHECK (length(channel) BETWEEN 1 AND 64),
    scope_kind TEXT NOT NULL CHECK (length(scope_kind) BETWEEN 1 AND 64),
    scope_id TEXT NOT NULL CHECK (length(scope_id) BETWEEN 1 AND 256),
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 71),
    payload_json BYTEA NOT NULL CHECK (octet_length(payload_json) <= 1048576),
    impact_rows BIGINT NOT NULL CHECK (impact_rows >= 0),
    impact_resources_json JSONB NOT NULL CHECK (jsonb_typeof(impact_resources_json) = 'array' AND jsonb_array_length(impact_resources_json) <= 32 AND octet_length(impact_resources_json::text) <= 1048576),
    snapshot_hash TEXT NOT NULL CHECK (length(snapshot_hash) IN (0, 71)),
    decision_fingerprint TEXT NOT NULL CHECK (length(decision_fingerprint) BETWEEN 1 AND 256),
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    expires_at TEXT NOT NULL CHECK (length(expires_at) BETWEEN 20 AND 30),
    expires_at_unix_nano BIGINT NOT NULL
);

CREATE INDEX modary_action_plan_expiry_idx ON modary_action_plan (expires_at_unix_nano);

CREATE TABLE modary_action_idempotency (
    scope_kind TEXT NOT NULL CHECK (length(scope_kind) BETWEEN 1 AND 64),
    scope_id TEXT NOT NULL CHECK (length(scope_id) BETWEEN 1 AND 256),
    actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 256),
    actor_type TEXT NOT NULL CHECK (length(actor_type) BETWEEN 1 AND 64),
    action_id TEXT NOT NULL CHECK (length(action_id) BETWEEN 1 AND 127),
    action_version TEXT NOT NULL CHECK (length(action_version) BETWEEN 1 AND 128),
    contract_hash TEXT NOT NULL CHECK (length(contract_hash) = 71),
    channel TEXT NOT NULL CHECK (length(channel) BETWEEN 1 AND 64),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 256),
    input_hash TEXT NOT NULL CHECK (length(input_hash) = 71),
    plan_hash TEXT NOT NULL CHECK (length(plan_hash) = 71),
    impact_rows BIGINT NOT NULL CHECK (impact_rows >= 0),
    impact_resources_json JSONB NOT NULL CHECK (jsonb_typeof(impact_resources_json) = 'array' AND jsonb_array_length(impact_resources_json) <= 32 AND octet_length(impact_resources_json::text) <= 1048576),
    decision_fingerprint TEXT NOT NULL CHECK (length(decision_fingerprint) BETWEEN 1 AND 256),
    status TEXT NOT NULL CHECK (status IN ('running', 'completed')),
    result_data_json BYTEA CHECK (result_data_json IS NULL OR octet_length(result_data_json) <= 1048576),
    result_summary TEXT NOT NULL CHECK (length(result_summary) <= 512),
    result_references_json JSONB NOT NULL CHECK (jsonb_typeof(result_references_json) = 'array' AND jsonb_array_length(result_references_json) <= 32 AND octet_length(result_references_json::text) <= 1048576),
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30),
    PRIMARY KEY (scope_kind, scope_id, actor_id, actor_type, action_id, idempotency_key),
    CHECK ((status = 'running' AND result_data_json IS NULL) OR (status = 'completed' AND result_data_json IS NOT NULL))
);
