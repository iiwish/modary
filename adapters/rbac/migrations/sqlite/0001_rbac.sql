CREATE TABLE modary_rbac_role (
    role_id TEXT PRIMARY KEY,
    max_rows INTEGER NOT NULL DEFAULT 0 CHECK (max_rows >= 0),
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE modary_rbac_role_permission (
    role_id TEXT NOT NULL REFERENCES modary_rbac_role(role_id) ON DELETE CASCADE,
    permission TEXT NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE modary_rbac_binding (
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    role_id TEXT NOT NULL REFERENCES modary_rbac_role(role_id) ON DELETE CASCADE,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (actor_id, actor_type, scope_kind, scope_id, role_id)
);

CREATE INDEX modary_rbac_binding_lookup
    ON modary_rbac_binding(actor_id, actor_type, scope_kind, scope_id, active);
