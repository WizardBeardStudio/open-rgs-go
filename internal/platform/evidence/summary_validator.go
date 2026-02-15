package evidence

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// ValidateSummaryJSON validates verify-evidence summary payloads.
func ValidateSummaryJSON(data []byte) error {
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	version, ok := s["summary_schema_version"].(float64)
	if !ok {
		return fmt.Errorf("field summary_schema_version must be number")
	}
	switch version {
	case 2:
		return validateSummarySchema2(s)
	default:
		return fmt.Errorf("unsupported summary_schema_version: %v", version)
	}
}

func validateSummarySchema2(s map[string]any) error {
	if err := requireNumberEquals(s, "summary_schema_version", 2); err != nil {
		return err
	}
	if err := requireNonEmptyString(s, "timestamp_utc"); err != nil {
		return err
	}
	if err := requireNonEmptyString(s, "run_dir"); err != nil {
		return err
	}
	if err := requireInSetString(s, "proto_mode", "full", "diff-only"); err != nil {
		return err
	}
	if err := requireBool(s, "require_clean_worktree"); err != nil {
		return err
	}
	if err := requireBool(s, "required_artifacts_present"); err != nil {
		return err
	}
	if err := requireBool(s, "optional_changed_files_present"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "required_artifact_count_expected"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "required_artifact_count_present"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "required_artifact_count_missing"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "artifact_file_count"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "artifact_total_bytes"); err != nil {
		return err
	}
	if err := requireNumberEquals(s, "summary_validation_status", 0); err != nil {
		return err
	}
	if err := requireNonEmptyString(s, "summary_validation_log"); err != nil {
		return err
	}
	if err := requireSHA256HexString(s, "summary_validation_log_sha256"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "proto_check_status"); err != nil {
		return err
	}
	if err := requireNumberNonNegative(s, "make_verify_status"); err != nil {
		return err
	}
	if err := requireInSetString(s, "overall_status", "pass", "fail"); err != nil {
		return err
	}
	if err := requireOptionalInSetString(s, "failed_step", "proto_check", "make_verify", "both"); err != nil {
		return err
	}
	if err := requireOptionalInSetString(s, "changed_files_artifact", "changed_files.txt"); err != nil {
		return err
	}

	expected := int(numberValue(s["required_artifact_count_expected"]))
	present := int(numberValue(s["required_artifact_count_present"]))
	missing := int(numberValue(s["required_artifact_count_missing"]))
	if expected < 0 || present < 0 || missing < 0 {
		return fmt.Errorf("required artifact counts must be non-negative")
	}
	if present+missing != expected {
		return fmt.Errorf("required artifact counts inconsistent: present(%d)+missing(%d)!=expected(%d)", present, missing, expected)
	}
	requiredPresent := s["required_artifacts_present"].(bool)
	if requiredPresent != (missing == 0) {
		return fmt.Errorf("required_artifacts_present inconsistent with missing count")
	}

	optionalChanged := s["optional_changed_files_present"].(bool)
	changedArtifact := s["changed_files_artifact"]
	if optionalChanged && changedArtifact != "changed_files.txt" {
		return fmt.Errorf("optional_changed_files_present=true requires changed_files_artifact=changed_files.txt")
	}
	if !optionalChanged && changedArtifact != nil {
		return fmt.Errorf("optional_changed_files_present=false requires changed_files_artifact=null")
	}

	protoStatus := int(numberValue(s["proto_check_status"]))
	verifyStatus := int(numberValue(s["make_verify_status"]))
	overall := s["overall_status"].(string)
	failedStep := s["failed_step"]
	if overall == "pass" {
		if protoStatus != 0 || verifyStatus != 0 {
			return fmt.Errorf("overall_status=pass requires zero statuses")
		}
		if failedStep != nil {
			return fmt.Errorf("overall_status=pass requires failed_step=null")
		}
	} else {
		if protoStatus == 0 && verifyStatus == 0 {
			return fmt.Errorf("overall_status=fail requires non-zero status")
		}
		want := failureLabel(protoStatus, verifyStatus)
		if failedStep != want {
			return fmt.Errorf("failed_step mismatch: got=%v want=%v", failedStep, want)
		}
	}

	return nil
}

func failureLabel(protoStatus, verifyStatus int) any {
	switch {
	case protoStatus != 0 && verifyStatus != 0:
		return "both"
	case protoStatus != 0:
		return "proto_check"
	default:
		return "make_verify"
	}
}

func requireNonEmptyString(m map[string]any, key string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return fmt.Errorf("field %s must be non-empty string", key)
	}
	return nil
}

func requireInSetString(m map[string]any, key string, vals ...string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("field %s must be string", key)
	}
	for _, item := range vals {
		if s == item {
			return nil
		}
	}
	return fmt.Errorf("field %s has invalid value: %q", key, s)
}

func requireOptionalInSetString(m map[string]any, key string, vals ...string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("field %s must be string or null", key)
	}
	for _, item := range vals {
		if s == item {
			return nil
		}
	}
	return fmt.Errorf("field %s has invalid value: %q", key, s)
}

func requireBool(m map[string]any, key string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	if _, ok := v.(bool); !ok {
		return fmt.Errorf("field %s must be bool", key)
	}
	return nil
}

func requireNumberEquals(m map[string]any, key string, n float64) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	f, ok := v.(float64)
	if !ok || f != n {
		return fmt.Errorf("field %s must equal %v", key, n)
	}
	return nil
}

func requireNumberNonNegative(m map[string]any, key string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	f, ok := v.(float64)
	if !ok || f < 0 {
		return fmt.Errorf("field %s must be non-negative number", key)
	}
	return nil
}

var sha256HexPattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

func requireSHA256HexString(m map[string]any, key string) error {
	v, ok := m[key]
	if !ok {
		return fmt.Errorf("missing field: %s", key)
	}
	s, ok := v.(string)
	if !ok || !sha256HexPattern.MatchString(s) {
		return fmt.Errorf("field %s must be 64-char hex sha256", key)
	}
	return nil
}

func numberValue(v any) float64 {
	f, _ := v.(float64)
	return f
}
