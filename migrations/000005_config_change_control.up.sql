CREATE TYPE config_change_status AS ENUM (
    'proposed',
    'approved',
    'applied',
    'rejected'
);

CREATE TYPE download_action AS ENUM (
    'add',
    'update',
    'delete',
    'activate'
);

CREATE TABLE IF NOT EXISTS config_changes (
    change_id TEXT PRIMARY KEY,
    config_namespace TEXT NOT NULL,
    config_key TEXT NOT NULL,
    proposed_value TEXT NOT NULL,
    previous_value TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    status config_change_status NOT NULL,
    proposer_id TEXT NOT NULL,
    approver_id TEXT NOT NULL DEFAULT '',
    applied_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    request_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_config_changes_namespace_key_time
    ON config_changes(config_namespace, config_key, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_config_changes_status_time
    ON config_changes(status, created_at DESC);

CREATE TABLE IF NOT EXISTS config_current_values (
    config_namespace TEXT NOT NULL,
    config_key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (config_namespace, config_key)
);

CREATE TABLE IF NOT EXISTS download_library_changes (
    entry_id TEXT PRIMARY KEY,
    library_path TEXT NOT NULL,
    checksum TEXT NOT NULL,
    version TEXT NOT NULL,
    action download_action NOT NULL,
    changed_by TEXT NOT NULL,
    reason TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_download_library_changes_path_time
    ON download_library_changes(library_path, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_download_library_changes_action_time
    ON download_library_changes(action, occurred_at DESC);
