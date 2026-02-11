DROP TRIGGER IF EXISTS tr_no_delete_audit_events ON audit_events;
DROP TRIGGER IF EXISTS tr_no_update_audit_events ON audit_events;
DROP FUNCTION IF EXISTS prevent_audit_mutation();
DROP TABLE IF EXISTS audit_events;
