CREATE TABLE modary_identity_principal (
    actor_id TEXT PRIMARY KEY,
    actor_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE modary_identity_password (
    username TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL UNIQUE REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE modary_identity_bearer (
    token_id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    actor_id TEXT NOT NULL REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE modary_identity_session (
    session_id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    actor_id TEXT NOT NULL REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX modary_identity_session_expiry
    ON modary_identity_session(expires_at);
