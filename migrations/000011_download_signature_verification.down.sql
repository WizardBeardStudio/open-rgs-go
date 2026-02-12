DROP INDEX IF EXISTS idx_download_library_changes_signer_time;

ALTER TABLE download_library_changes
  DROP COLUMN IF EXISTS signature_alg,
  DROP COLUMN IF EXISTS signature,
  DROP COLUMN IF EXISTS signer_kid;
