DROP TRIGGER IF EXISTS tr_equipment_registry_updated_at ON equipment_registry;
DROP FUNCTION IF EXISTS set_equipment_registry_updated_at();

DROP TABLE IF EXISTS ingestion_buffer_audit;
DROP TABLE IF EXISTS ingestion_buffers;
DROP TABLE IF EXISTS meter_records;
DROP TABLE IF EXISTS significant_events;
DROP TABLE IF EXISTS equipment_registry;

DROP TYPE IF EXISTS ingestion_buffer_status;
DROP TYPE IF EXISTS ingestion_record_kind;
DROP TYPE IF EXISTS equipment_status;
