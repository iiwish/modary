CREATE TABLE consumer_counter (
    scope_kind TEXT NOT NULL CHECK (length(scope_kind) BETWEEN 1 AND 64),
    scope_id TEXT NOT NULL CHECK (length(scope_id) BETWEEN 1 AND 256),
    value BIGINT NOT NULL,
    version BIGINT NOT NULL CHECK (version >= 1),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30),
    PRIMARY KEY (scope_kind, scope_id)
);
