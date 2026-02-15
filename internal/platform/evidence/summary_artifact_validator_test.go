package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSummaryArtifactStrict(t *testing.T) {
	t.Parallel()

	summaryPath, summary := writeValidSummaryArtifact(t)
	if err := ValidateSummaryArtifact(summaryPath, "strict"); err != nil {
		t.Fatalf("expected strict validation to pass: %v", err)
	}

	t.Run("missing validation log", func(t *testing.T) {
		t.Parallel()
		path, s := writeValidSummaryArtifact(t)
		if err := os.Remove(filepath.Join(filepath.Dir(path), "summary_validation.log")); err != nil {
			t.Fatalf("remove summary_validation.log: %v", err)
		}
		if err := ValidateSummaryArtifact(path, "strict"); err == nil {
			t.Fatalf("expected error when summary_validation.log is missing")
		}
		_ = s
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		t.Parallel()
		path, s := writeValidSummaryArtifact(t)
		s["summary_validation_log_sha256"] = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		writeSummary(t, path, s)
		if err := ValidateSummaryArtifact(path, "strict"); err == nil {
			t.Fatalf("expected checksum mismatch error")
		}
	})

	t.Run("stale index", func(t *testing.T) {
		t.Parallel()
		path, _ := writeValidSummaryArtifact(t)
		idx := filepath.Join(filepath.Dir(path), "index.txt")
		if err := os.WriteFile(idx, []byte("verify evidence artifact index\n"), 0o644); err != nil {
			t.Fatalf("write stale index: %v", err)
		}
		if err := ValidateSummaryArtifact(path, "strict"); err == nil {
			t.Fatalf("expected stale index error")
		}
	})

	t.Run("bad counters", func(t *testing.T) {
		t.Parallel()
		path, s := writeValidSummaryArtifact(t)
		s["required_artifact_count_expected"] = 5
		s["required_artifact_count_present"] = 4
		s["required_artifact_count_missing"] = 0
		writeSummary(t, path, s)
		if err := ValidateSummaryArtifact(path, "strict"); err == nil {
			t.Fatalf("expected counter consistency error")
		}
	})

	_ = summary
}

func writeValidSummaryArtifact(t *testing.T) (string, map[string]any) {
	t.Helper()

	runDir := filepath.Join(t.TempDir(), "artifacts", "verify", "20260215T000000Z")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	logContent := []byte("verify summary validation passed\n")
	logPath := filepath.Join(runDir, "summary_validation.log")
	if err := os.WriteFile(logPath, logContent, 0o644); err != nil {
		t.Fatalf("write summary_validation.log: %v", err)
	}
	logSum := sha256.Sum256(logContent)
	logSHA := hex.EncodeToString(logSum[:])

	indexContent := "verify evidence artifact index\nsummary_validation.log\t33\n"
	if err := os.WriteFile(filepath.Join(runDir, "index.txt"), []byte(indexContent), 0o644); err != nil {
		t.Fatalf("write index.txt: %v", err)
	}

	manifestContent := logSHA + "  " + filepath.Join(runDir, "summary_validation.log") + "\n"
	if err := os.WriteFile(filepath.Join(runDir, "manifest.sha256"), []byte(manifestContent), 0o644); err != nil {
		t.Fatalf("write manifest.sha256: %v", err)
	}

	summary := map[string]any{
		"summary_schema_version":           2,
		"timestamp_utc":                    "2026-02-15T00:00:00Z",
		"run_dir":                          runDir,
		"proto_mode":                       "full",
		"require_clean_worktree":           true,
		"required_artifacts_present":       true,
		"optional_changed_files_present":   false,
		"required_artifact_count_expected": 5,
		"required_artifact_count_present":  5,
		"required_artifact_count_missing":  0,
		"artifact_file_count":              5,
		"artifact_total_bytes":             1000,
		"summary_validation_status":        0,
		"summary_validation_log":           "summary_validation.log",
		"summary_validation_log_sha256":    logSHA,
		"proto_check_status":               0,
		"make_verify_status":               0,
		"overall_status":                   "pass",
		"failed_step":                      nil,
		"changed_files_artifact":           nil,
	}

	summaryPath := filepath.Join(runDir, "summary.json")
	writeSummary(t, summaryPath, summary)
	return summaryPath, summary
}

func writeSummary(t *testing.T, path string, payload map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}
