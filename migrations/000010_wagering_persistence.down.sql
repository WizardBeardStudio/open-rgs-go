DROP INDEX IF EXISTS idx_wagering_idempotency_expires;
DROP TABLE IF EXISTS wagering_idempotency_keys;

DROP INDEX IF EXISTS idx_wagers_status_time;
DROP INDEX IF EXISTS idx_wagers_player_time;
DROP TABLE IF EXISTS wagers;
