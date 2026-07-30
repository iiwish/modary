CREATE TABLE IF NOT EXISTS rulary_workspace (
    workspace_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rulary_ruleset (
    ruleset_id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES rulary_workspace(workspace_id),
    name TEXT NOT NULL,
    draft_spec BLOB NOT NULL,
    draft_hash TEXT NOT NULL,
    validated_hash TEXT,
    state TEXT NOT NULL CHECK (state IN ('draft', 'published')),
    current_version_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rulary_ruleset_version (
    version_id TEXT PRIMARY KEY,
    ruleset_id TEXT NOT NULL REFERENCES rulary_ruleset(ruleset_id),
    workspace_id TEXT NOT NULL,
    version_number INTEGER NOT NULL,
    spec_json BLOB NOT NULL,
    spec_hash TEXT NOT NULL,
    published_by TEXT NOT NULL,
    published_at TEXT NOT NULL,
    UNIQUE (ruleset_id, version_number)
);

CREATE TRIGGER IF NOT EXISTS rulary_version_immutable_update
BEFORE UPDATE ON rulary_ruleset_version
BEGIN
    SELECT RAISE(ABORT, 'published RuleVersion is immutable');
END;

CREATE TRIGGER IF NOT EXISTS rulary_version_immutable_delete
BEFORE DELETE ON rulary_ruleset_version
BEGIN
    SELECT RAISE(ABORT, 'published RuleVersion is immutable');
END;

CREATE TABLE IF NOT EXISTS rulary_run (
    run_id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    rule_version_id TEXT NOT NULL REFERENCES rulary_ruleset_version(version_id),
    status TEXT NOT NULL,
    matched_rows INTEGER NOT NULL,
    written_rows INTEGER NOT NULL,
    rejected_rows INTEGER NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rulary_label_result (
    result_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES rulary_run(run_id),
    company_id TEXT NOT NULL,
    label_json BLOB NOT NULL,
    evidence_json BLOB NOT NULL,
    processed_at TEXT NOT NULL,
    UNIQUE (run_id, company_id)
);

CREATE TABLE IF NOT EXISTS company_license (
    company_id TEXT PRIMARY KEY,
    company_name TEXT NOT NULL,
    license_address TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS company_address_labels (
    company_id TEXT PRIMARY KEY REFERENCES company_license(company_id),
    registered_address TEXT NOT NULL,
    business_address TEXT NOT NULL,
    address_note TEXT NOT NULL,
    has_business_address_filing INTEGER NOT NULL,
    address_quality_tag TEXT NOT NULL,
    rule_version INTEGER NOT NULL,
    run_id TEXT NOT NULL,
    evidence_json BLOB NOT NULL,
    processed_at TEXT NOT NULL
);
