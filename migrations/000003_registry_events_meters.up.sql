CREATE TYPE equipment_status AS ENUM (
    'active',
    'inactive',
    'maintenance',
    'disabled',
    'retired'
);

CREATE TYPE ingestion_record_kind AS ENUM (
    'significant_event',
    'meter_snapshot',
    'meter_delta'
);

CREATE TYPE ingestion_buffer_status AS ENUM (
    'queued',
    'processing',
    'acknowledged',
    'dead_letter'
);

CREATE TABLE IF NOT EXISTS equipment_registry (
    equipment_id TEXT PRIMARY KEY,
    external_reference TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    status equipment_status NOT NULL DEFAULT 'active',
    theoretical_rtp_bps INTEGER CHECK (theoretical_rtp_bps >= 0 AND theoretical_rtp_bps <= 10000),
    control_program_version TEXT NOT NULL DEFAULT '',
    config_version TEXT NOT NULL DEFAULT '',
    attributes JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_equipment_registry_status
    ON equipment_registry(status);

CREATE INDEX IF NOT EXISTS idx_equipment_registry_location
    ON equipment_registry(location);

CREATE TABLE IF NOT EXISTS significant_events (
    event_id TEXT PRIMARY KEY,
    equipment_id TEXT NOT NULL REFERENCES equipment_registry(equipment_id),
    event_code TEXT NOT NULL,
    localized_description TEXT NOT NULL,
    severity TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_event_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '{}'::JSONB,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_significant_events_source
    ON significant_events(equipment_id, source_event_id)
    WHERE source_event_id <> '';

CREATE INDEX IF NOT EXISTS idx_significant_events_equipment_time
    ON significant_events(equipment_id, recorded_at DESC);

CREATE INDEX IF NOT EXISTS idx_significant_events_code_time
    ON significant_events(event_code, recorded_at DESC);

CREATE TABLE IF NOT EXISTS meter_records (
    meter_id TEXT PRIMARY KEY,
    equipment_id TEXT NOT NULL REFERENCES equipment_registry(equipment_id),
    meter_label TEXT NOT NULL,
    monetary_unit CHAR(3) NOT NULL,
    record_kind ingestion_record_kind NOT NULL,
    value_minor BIGINT NOT NULL DEFAULT 0,
    delta_minor BIGINT NOT NULL DEFAULT 0,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_meter_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT '',
    tags JSONB NOT NULL DEFAULT '{}'::JSONB,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_meter_records_source
    ON meter_records(equipment_id, meter_label, source_meter_id)
    WHERE source_meter_id <> '';

CREATE INDEX IF NOT EXISTS idx_meter_records_equipment_label_time
    ON meter_records(equipment_id, meter_label, recorded_at DESC);

CREATE INDEX IF NOT EXISTS idx_meter_records_kind_time
    ON meter_records(record_kind, recorded_at DESC);

CREATE TABLE IF NOT EXISTS ingestion_buffers (
    buffer_id BIGSERIAL PRIMARY KEY,
    record_kind ingestion_record_kind NOT NULL,
    status ingestion_buffer_status NOT NULL DEFAULT 'queued',
    equipment_id TEXT NOT NULL,
    source_record_id TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_attempt_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    failure_reason TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    CONSTRAINT fk_ingestion_buffers_equipment
        FOREIGN KEY (equipment_id) REFERENCES equipment_registry(equipment_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ingestion_buffers_request_kind
    ON ingestion_buffers(request_id, record_kind)
    WHERE request_id <> '';

CREATE INDEX IF NOT EXISTS idx_ingestion_buffers_status_next_attempt
    ON ingestion_buffers(status, next_attempt_at, queued_at);

CREATE INDEX IF NOT EXISTS idx_ingestion_buffers_equipment_queued
    ON ingestion_buffers(equipment_id, queued_at DESC);

CREATE TABLE IF NOT EXISTS ingestion_buffer_audit (
    buffer_audit_id BIGSERIAL PRIMARY KEY,
    buffer_id BIGINT NOT NULL REFERENCES ingestion_buffers(buffer_id),
    previous_status ingestion_buffer_status,
    new_status ingestion_buffer_status NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL DEFAULT '',
    actor_type TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_ingestion_buffer_audit_buffer_time
    ON ingestion_buffer_audit(buffer_id, changed_at DESC);

CREATE OR REPLACE FUNCTION set_equipment_registry_updated_at()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_equipment_registry_updated_at ON equipment_registry;
CREATE TRIGGER tr_equipment_registry_updated_at
BEFORE UPDATE ON equipment_registry
FOR EACH ROW
EXECUTE FUNCTION set_equipment_registry_updated_at();
