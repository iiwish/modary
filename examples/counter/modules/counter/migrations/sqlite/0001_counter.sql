CREATE TABLE IF NOT EXISTS consumer_counter (
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    value INTEGER NOT NULL,
    version INTEGER NOT NULL CHECK (version >= 1),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (scope_kind, scope_id)
);
