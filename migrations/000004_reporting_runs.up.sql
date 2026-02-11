CREATE TYPE report_run_status AS ENUM (
    'completed',
    'failed'
);

CREATE TABLE IF NOT EXISTS report_runs (
    report_run_id TEXT PRIMARY KEY,
    report_type TEXT NOT NULL,
    report_interval TEXT NOT NULL,
    report_format TEXT NOT NULL,
    status report_run_status NOT NULL,
    operator_id TEXT NOT NULL,
    report_title TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    no_activity BOOLEAN NOT NULL DEFAULT FALSE,
    content_type TEXT NOT NULL,
    content BYTEA NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_report_runs_generated_at
    ON report_runs(generated_at DESC);

CREATE INDEX IF NOT EXISTS idx_report_runs_type_interval
    ON report_runs(report_type, report_interval, generated_at DESC);

CREATE INDEX IF NOT EXISTS idx_report_runs_operator_time
    ON report_runs(operator_id, generated_at DESC);
