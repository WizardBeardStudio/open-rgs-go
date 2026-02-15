package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateSummaryArtifact validates summary.json plus linked artifact files.
// mode accepts "json" (schema/consistency only) or "strict" (with file checks).
func ValidateSummaryArtifact(summaryPath, mode string) error {
	if mode != "json" && mode != "strict" {
		return fmt.Errorf("invalid validation mode: %s", mode)
	}

	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("read summary: %w", err)
	}
	if err := ValidateSummaryJSON(data); err != nil {
		return err
	}
	if mode == "json" {
		return nil
	}

	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	runDir, _ := s["run_dir"].(string)
	logName, _ := s["summary_validation_log"].(string)
	expectedLogSHA, _ := s["summary_validation_log_sha256"].(string)

	if filepath.Base(logName) != logName {
		return fmt.Errorf("summary_validation_log must be basename, got: %s", logName)
	}

	summaryDir := filepath.Clean(filepath.Dir(summaryPath))
	if filepath.Clean(runDir) != summaryDir {
		return fmt.Errorf("run_dir mismatch: summary has %q file dir is %q", runDir, summaryDir)
	}

	logPath := filepath.Join(summaryDir, logName)
	logData, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("read summary validation log: %w", err)
	}
	actualLogSHA := sha256.Sum256(logData)
	actualLogSHAHex := hex.EncodeToString(actualLogSHA[:])
	if !strings.EqualFold(actualLogSHAHex, expectedLogSHA) {
		return fmt.Errorf("summary_validation_log_sha256 mismatch: expected=%s actual=%s", expectedLogSHA, actualLogSHAHex)
	}

	indexPath := filepath.Join(summaryDir, "index.txt")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read index.txt: %w", err)
	}
	if !strings.Contains(string(indexData), "summary_validation.log\t") {
		return fmt.Errorf("index.txt missing summary_validation.log entry")
	}

	manifestPath := filepath.Join(summaryDir, "manifest.sha256")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest.sha256: %w", err)
	}

	wantManifestPath := filepath.Clean(filepath.Join(runDir, logName))
	foundManifestLine := false
	for _, line := range strings.Split(string(manifestData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := fields[0]
		path := filepath.Clean(fields[1])
		if strings.EqualFold(sum, expectedLogSHA) && path == wantManifestPath {
			foundManifestLine = true
			break
		}
	}
	if !foundManifestLine {
		return fmt.Errorf("manifest.sha256 missing summary_validation.log checksum line")
	}

	return nil
}
