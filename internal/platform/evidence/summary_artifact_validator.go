package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultVerifyEvidenceAttestationKey = "open-rgs-go-dev-attestation-key"

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
	attestationFile, _ := s["attestation_file"].(string)
	attestationSigFile, _ := s["attestation_signature_file"].(string)

	if filepath.Base(logName) != logName {
		return fmt.Errorf("summary_validation_log must be basename, got: %s", logName)
	}
	if filepath.Base(attestationFile) != attestationFile {
		return fmt.Errorf("attestation_file must be basename, got: %s", attestationFile)
	}
	if filepath.Base(attestationSigFile) != attestationSigFile {
		return fmt.Errorf("attestation_signature_file must be basename, got: %s", attestationSigFile)
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

	attestationPath := filepath.Join(summaryDir, attestationFile)
	attestationData, err := os.ReadFile(attestationPath)
	if err != nil {
		return fmt.Errorf("read attestation: %w", err)
	}
	attestationSigPath := filepath.Join(summaryDir, attestationSigFile)
	attestationSigData, err := os.ReadFile(attestationSigPath)
	if err != nil {
		return fmt.Errorf("read attestation signature: %w", err)
	}

	attestationSig := strings.TrimSpace(string(attestationSigData))
	key := os.Getenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY")
	enforceKey := os.Getenv("RGS_VERIFY_EVIDENCE_ENFORCE_ATTESTATION_KEY") == "true" || os.Getenv("GITHUB_ACTIONS") == "true"
	if enforceKey {
		if key == "" {
			return fmt.Errorf("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY must be set in strict/CI validation")
		}
		if key == DefaultVerifyEvidenceAttestationKey {
			return fmt.Errorf("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY must not use default development key in strict/CI validation")
		}
		if len(key) < 32 {
			return fmt.Errorf("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY must be at least 32 characters in strict/CI validation")
		}
	}
	if key == "" {
		key = DefaultVerifyEvidenceAttestationKey
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(attestationData)
	wantSig := hex.EncodeToString(mac.Sum(nil))
	if !strings.EqualFold(attestationSig, wantSig) {
		return fmt.Errorf("attestation signature mismatch")
	}

	var a map[string]any
	if err := json.Unmarshal(attestationData, &a); err != nil {
		return fmt.Errorf("invalid attestation JSON: %w", err)
	}
	if err := requireNumberEquals(a, "attestation_schema_version", 1); err != nil {
		return err
	}
	if err := requireNonEmptyString(a, "generated_at"); err != nil {
		return err
	}
	if err := requireNonEmptyString(a, "run_dir"); err != nil {
		return err
	}
	if err := requireNonEmptyString(a, "summary_sha256"); err != nil {
		return err
	}
	attRunDir, _ := a["run_dir"].(string)
	if filepath.Clean(attRunDir) != filepath.Clean(runDir) {
		return fmt.Errorf("attestation run_dir mismatch")
	}

	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		return fmt.Errorf("read summary for attestation hash: %w", err)
	}
	summarySHA := sha256.Sum256(summaryData)
	summarySHAHex := hex.EncodeToString(summarySHA[:])
	wantSummarySHA, _ := a["summary_sha256"].(string)
	if !strings.EqualFold(summarySHAHex, wantSummarySHA) {
		return fmt.Errorf("attestation summary_sha256 mismatch")
	}

	if !strings.Contains(string(indexData), "attestation.json\t") || !strings.Contains(string(indexData), "attestation.sig\t") {
		return fmt.Errorf("index.txt missing attestation artifacts")
	}
	attestationSHA := sha256.Sum256(attestationData)
	attestationSHAHex := hex.EncodeToString(attestationSHA[:])
	attestationSigSHA := sha256.Sum256(attestationSigData)
	attestationSigSHAHex := hex.EncodeToString(attestationSigSHA[:])
	wantAttPath := filepath.Clean(filepath.Join(runDir, attestationFile))
	wantSigPath := filepath.Clean(filepath.Join(runDir, attestationSigFile))
	foundAttLine := false
	foundSigLine := false
	for _, line := range strings.Split(string(manifestData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := fields[0]
		path := filepath.Clean(fields[1])
		if strings.EqualFold(sum, attestationSHAHex) && path == wantAttPath {
			foundAttLine = true
		}
		if strings.EqualFold(sum, attestationSigSHAHex) && path == wantSigPath {
			foundSigLine = true
		}
	}
	if !foundAttLine || !foundSigLine {
		return fmt.Errorf("manifest.sha256 missing attestation checksum entries")
	}

	return nil
}
