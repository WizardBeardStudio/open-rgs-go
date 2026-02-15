package evidence

import (
	"strings"
	"testing"
)

const validSummary = `{
  "summary_schema_version": 2,
  "timestamp_utc": "2026-02-15T00:00:00Z",
  "run_dir": "artifacts/verify/20260215T000000Z",
  "git_commit": "abc",
  "git_branch": "main",
  "git_describe": "v0.1.0-1-gabc",
  "git_worktree_clean_before": true,
  "git_changed_files_count_before": 0,
  "git_worktree_clean_after": true,
  "git_changed_files_count_after": 0,
  "git_worktree_clean": true,
  "git_changed_files_count": 0,
  "proto_mode": "full",
  "require_clean_worktree": true,
  "github_actions": true,
  "ci_run_id": "1",
  "ci_run_attempt": "1",
  "ci_ref": "refs/heads/main",
  "ci_sha": "abc",
  "hostname": "host",
  "os": "Linux",
  "arch": "x86_64",
  "go_version": "go version go1.24.0 linux/amd64",
  "buf_version": "1.64.0",
  "go_mod_sha256": "x",
  "go_sum_sha256": "x",
  "check_module_path_script_sha256": "x",
  "check_proto_clean_script_sha256": "x",
  "verify_evidence_script_sha256": "x",
  "makefile_sha256": "x",
  "ci_workflow_sha256": "x",
  "proto_check_command": "make proto-check",
  "proto_check_started_at": "2026-02-15T00:00:00Z",
  "proto_check_finished_at": "2026-02-15T00:00:01Z",
  "proto_check_duration_seconds": 1,
  "make_verify_command": "make verify",
  "make_verify_started_at": "2026-02-15T00:00:01Z",
  "make_verify_finished_at": "2026-02-15T00:00:02Z",
  "make_verify_duration_seconds": 1,
  "proto_check_status": 0,
  "make_verify_status": 0,
  "overall_status": "pass",
  "failed_step": null,
  "changed_files_artifact": null,
  "required_artifacts_present": true,
  "required_artifact_count_expected": 4,
  "required_artifact_count_present": 4,
  "required_artifact_count_missing": 0,
  "optional_changed_files_present": false,
  "artifact_file_count": 5,
  "artifact_total_bytes": 1000
}`

func TestValidateSummaryJSONPass(t *testing.T) {
	if err := ValidateSummaryJSON([]byte(validSummary)); err != nil {
		t.Fatalf("expected valid summary, err=%v", err)
	}
}

func TestValidateSummaryJSONFailsSchemaVersion(t *testing.T) {
	bad := replace(validSummary, `"summary_schema_version": 2`, `"summary_schema_version": 1`)
	if err := ValidateSummaryJSON([]byte(bad)); err == nil {
		t.Fatalf("expected schema version error")
	}
}

func TestValidateSummaryJSONFailsStatusConsistency(t *testing.T) {
	bad := replace(validSummary, `"overall_status": "pass"`, `"overall_status": "fail"`)
	if err := ValidateSummaryJSON([]byte(bad)); err == nil {
		t.Fatalf("expected status consistency error")
	}
}

func TestValidateSummaryJSONFailsRequiredCounts(t *testing.T) {
	bad := replace(validSummary, `"required_artifact_count_present": 4`, `"required_artifact_count_present": 3`)
	if err := ValidateSummaryJSON([]byte(bad)); err == nil {
		t.Fatalf("expected required count consistency error")
	}
}

func replace(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}
