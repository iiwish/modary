CREATE TABLE modary_identity_principal (
    actor_id TEXT PRIMARY KEY CHECK (length(actor_id) BETWEEN 1 AND 256),
    actor_type TEXT NOT NULL CHECK (length(actor_type) BETWEEN 1 AND 64),
    display_name TEXT NOT NULL CHECK (length(display_name) <= 256),
    scope_kind TEXT NOT NULL CHECK (length(scope_kind) BETWEEN 1 AND 64),
    scope_id TEXT NOT NULL CHECK (length(scope_id) BETWEEN 1 AND 256),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30)
);

CREATE TABLE modary_identity_password (
    username TEXT PRIMARY KEY CHECK (length(username) BETWEEN 1 AND 256),
    actor_id TEXT NOT NULL UNIQUE REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL CHECK (length(password_hash) = 97),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30)
);

CREATE TABLE modary_identity_bearer (
    token_id TEXT PRIMARY KEY CHECK (length(token_id) BETWEEN 1 AND 128),
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 71),
    actor_id TEXT NOT NULL REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30)
);

CREATE TABLE modary_identity_session (
    session_id TEXT PRIMARY KEY CHECK (length(session_id) BETWEEN 1 AND 128),
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) = 71),
    actor_id TEXT NOT NULL REFERENCES modary_identity_principal(actor_id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL CHECK (length(csrf_token) = 64),
    expires_at TEXT NOT NULL CHECK (length(expires_at) BETWEEN 20 AND 30),
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30)
);

CREATE INDEX modary_identity_session_expiry ON modary_identity_session(expires_at);
