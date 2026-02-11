-- Core append-only audit log with hash chaining by day partition key.
CREATE TABLE IF NOT EXISTS audit_events (
    audit_id TEXT PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    auth_context JSONB NOT NULL DEFAULT '{}'::JSONB,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    action TEXT NOT NULL,
    before_state JSONB,
    after_state JSONB,
    result TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    partition_day DATE NOT NULL,
    hash_prev TEXT NOT NULL,
    hash_curr TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_partition_day_recorded
    ON audit_events(partition_day, recorded_at);

CREATE OR REPLACE FUNCTION prevent_audit_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_no_update_audit_events ON audit_events;
CREATE TRIGGER tr_no_update_audit_events
BEFORE UPDATE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_mutation();

DROP TRIGGER IF EXISTS tr_no_delete_audit_events ON audit_events;
CREATE TRIGGER tr_no_delete_audit_events
BEFORE DELETE ON audit_events
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_mutation();
