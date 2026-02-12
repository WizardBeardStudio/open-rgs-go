ALTER TABLE download_library_changes
  ADD COLUMN IF NOT EXISTS signer_kid TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS signature TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS signature_alg TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_download_library_changes_signer_time
  ON download_library_changes(signer_kid, occurred_at DESC);
