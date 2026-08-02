CREATE TABLE modary_rbac_role (
    role_id TEXT PRIMARY KEY CHECK (length(role_id) BETWEEN 1 AND 127),
    max_rows BIGINT NOT NULL DEFAULT 0 CHECK (max_rows >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30)
);

CREATE TABLE modary_rbac_role_permission (
    role_id TEXT NOT NULL REFERENCES modary_rbac_role(role_id) ON DELETE CASCADE,
    permission TEXT NOT NULL CHECK (length(permission) BETWEEN 1 AND 127),
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE modary_rbac_binding (
    actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 256),
    actor_type TEXT NOT NULL CHECK (length(actor_type) BETWEEN 1 AND 64),
    scope_kind TEXT NOT NULL CHECK (length(scope_kind) BETWEEN 1 AND 64),
    scope_id TEXT NOT NULL CHECK (length(scope_id) BETWEEN 1 AND 256),
    role_id TEXT NOT NULL REFERENCES modary_rbac_role(role_id) ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TEXT NOT NULL CHECK (length(created_at) BETWEEN 20 AND 30),
    updated_at TEXT NOT NULL CHECK (length(updated_at) BETWEEN 20 AND 30),
    PRIMARY KEY (actor_id, actor_type, scope_kind, scope_id, role_id)
);

CREATE INDEX modary_rbac_binding_lookup ON modary_rbac_binding(actor_id, actor_type, scope_kind, scope_id, active);
