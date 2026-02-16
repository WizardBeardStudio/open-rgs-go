package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil, lookupMap(nil))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.keyID != "ci-active" {
		t.Fatalf("keyID = %q, want ci-active", cfg.keyID)
	}
	if cfg.format != "assignments" {
		t.Fatalf("format = %q, want assignments", cfg.format)
	}
	if cfg.outDir != "" {
		t.Fatalf("outDir = %q, want empty", cfg.outDir)
	}
	if !cfg.ringOutput {
		t.Fatalf("ringOutput = false, want true")
	}
	if cfg.privateMaterialFmt != "seed" {
		t.Fatalf("privateMaterialFmt = %q, want seed", cfg.privateMaterialFmt)
	}
}

func TestParseConfigEnvPrecedence(t *testing.T) {
	env := map[string]string{
		"RGS_ATTEST_KEYGEN_KEY_ID":                   "primary-id",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID":     "secondary-id",
		"RGS_ATTEST_KEYGEN_OUT_DIR":                  "/tmp/key-out",
		"RGS_ATTEST_KEYGEN_RING":                     "false",
		"RGS_ATTEST_KEYGEN_PRIVATE_MATERIAL":         "private",
		"RGS_ATTEST_KEYGEN_PRIVATE_VAR":              "PRIV",
		"RGS_ATTEST_KEYGEN_PUBLIC_VAR":               "PUB",
		"RGS_ATTEST_KEYGEN_EMIT_KEY_ID":              "no",
		"RGS_ATTEST_KEYGEN_KEY_ID_VAR":               "KEYID",
		"RGS_ATTEST_KEYGEN_ALG":                      "ed25519",
		"RGS_ATTEST_KEYGEN_PUBLIC_VAR_UNUSED_SPACES": "  ",
	}
	cfg, err := parseConfig(nil, lookupMap(env))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.keyID != "primary-id" {
		t.Fatalf("keyID = %q, want primary-id", cfg.keyID)
	}
	if cfg.outDir != "/tmp/key-out" {
		t.Fatalf("outDir = %q, want /tmp/key-out", cfg.outDir)
	}
	if cfg.ringOutput {
		t.Fatalf("ringOutput = true, want false")
	}
	if cfg.privateMaterialFmt != "private" {
		t.Fatalf("privateMaterialFmt = %q, want private", cfg.privateMaterialFmt)
	}
	if cfg.privateVar != "PRIV" || cfg.publicVar != "PUB" {
		t.Fatalf("vars = (%q,%q), want (PRIV,PUB)", cfg.privateVar, cfg.publicVar)
	}
	if cfg.emitKeyID {
		t.Fatalf("emitKeyID = true, want false")
	}
	if cfg.keyIDVar != "KEYID" {
		t.Fatalf("keyIDVar = %q, want KEYID", cfg.keyIDVar)
	}
}

func TestParseConfigFlagOverride(t *testing.T) {
	env := map[string]string{
		"RGS_ATTEST_KEYGEN_KEY_ID": "env-id",
	}
	cfg, err := parseConfig([]string{"--key-id", "flag-id", "--ring=false", "--format", "github-secrets"}, lookupMap(env))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.keyID != "flag-id" {
		t.Fatalf("keyID = %q, want flag-id", cfg.keyID)
	}
	if cfg.format != "github-secrets" {
		t.Fatalf("format = %q, want github-secrets", cfg.format)
	}
	if cfg.ringOutput {
		t.Fatalf("ringOutput = true, want false")
	}
}

func TestRenderAssignmentsRingSeed(t *testing.T) {
	cfg := config{
		keyID:              "ci-active",
		keyIDVar:           "KEY_ID",
		emitKeyID:          true,
		privateVar:         "PRIVATE",
		publicVar:          "PUBLIC",
		ringOutput:         true,
		privateMaterialFmt: "seed",
	}
	pub, priv := fixedKeyPair()
	lines := renderAssignments(cfg, renderKeyMaterial(cfg, pub, priv))
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if lines[0] != "KEY_ID=ci-active" {
		t.Fatalf("line[0] = %q", lines[0])
	}
	assertRingEntryLen(t, lines[1], "PRIVATE=ci-active:", ed25519.SeedSize)
	assertRingEntryLen(t, lines[2], "PUBLIC=ci-active:", ed25519.PublicKeySize)
}

func TestRenderAssignmentsSinglePrivateMaterial(t *testing.T) {
	cfg := config{
		keyID:              "ci-active",
		keyIDVar:           "KEY_ID",
		emitKeyID:          false,
		privateVar:         "PRIVATE",
		publicVar:          "PUBLIC",
		ringOutput:         false,
		privateMaterialFmt: "private",
	}
	pub, priv := fixedKeyPair()
	lines := renderAssignments(cfg, renderKeyMaterial(cfg, pub, priv))
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	assertValueLen(t, lines[0], "PRIVATE=", ed25519.PrivateKeySize)
	assertValueLen(t, lines[1], "PUBLIC=", ed25519.PublicKeySize)
}

func TestRenderAssignmentsGitHubSecretsFormat(t *testing.T) {
	cfg := config{
		format:             "github-secrets",
		keyID:              "ci-active",
		keyIDVar:           "KEY_ID",
		emitKeyID:          true,
		privateVar:         "PRIVATE",
		publicVar:          "PUBLIC",
		ringOutput:         true,
		privateMaterialFmt: "seed",
	}
	pub, priv := fixedKeyPair()
	lines := renderAssignments(cfg, renderKeyMaterial(cfg, pub, priv))
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if strings.HasPrefix(lines[0], "PRIVATE=") || strings.HasPrefix(lines[1], "PUBLIC=") || strings.HasPrefix(lines[0], "KEY_ID=") {
		t.Fatalf("github-secrets format should not contain assignments: %#v", lines)
	}
	assertValueLen(t, lines[0], "ci-active:", ed25519.SeedSize)
	assertValueLen(t, lines[1], "ci-active:", ed25519.PublicKeySize)
}

func TestWriteSecretFilesRingOutput(t *testing.T) {
	tmp := t.TempDir()
	cfg := config{
		keyID:      "ci-active",
		keyIDVar:   "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID",
		ringOutput: true,
		outDir:     tmp,
	}
	pub, priv := fixedKeyPair()
	lines, err := writeSecretFiles(cfg, renderKeyMaterial(config{
		keyID:              "ci-active",
		ringOutput:         true,
		privateMaterialFmt: "seed",
	}, pub, priv))
	if err != nil {
		t.Fatalf("writeSecretFiles() error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	privatePath := filepath.Join(tmp, "private-keys.txt")
	publicPath := filepath.Join(tmp, "public-keys.txt")
	assertFileMode600(t, privatePath)
	assertFileMode600(t, publicPath)
	assertFileContainsPrefix(t, privatePath, "ci-active:")
	assertFileContainsPrefix(t, publicPath, "ci-active:")
}

func TestWriteSecretFilesSingleOutput(t *testing.T) {
	tmp := t.TempDir()
	cfg := config{
		keyID:      "ci-active",
		keyIDVar:   "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID",
		ringOutput: false,
		outDir:     tmp,
	}
	pub, priv := fixedKeyPair()
	lines, err := writeSecretFiles(cfg, renderKeyMaterial(config{
		keyID:              "ci-active",
		ringOutput:         false,
		privateMaterialFmt: "private",
	}, pub, priv))
	if err != nil {
		t.Fatalf("writeSecretFiles() error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	privatePath := filepath.Join(tmp, "private-key.txt")
	publicPath := filepath.Join(tmp, "public-key.txt")
	assertFileMode600(t, privatePath)
	assertFileMode600(t, publicPath)
	assertFileBase64Len(t, privatePath, ed25519.PrivateKeySize)
	assertFileBase64Len(t, publicPath, ed25519.PublicKeySize)
}

func fixedKeyPair() (ed25519.PublicKey, ed25519.PrivateKey) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return pub, priv
}

func assertRingEntryLen(t *testing.T, line, prefix string, expectedLen int) {
	t.Helper()
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("line %q missing prefix %q", line, prefix)
	}
	raw := strings.TrimPrefix(line, prefix)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	if len(decoded) != expectedLen {
		t.Fatalf("decoded len = %d, want %d", len(decoded), expectedLen)
	}
}

func assertValueLen(t *testing.T, line, prefix string, expectedLen int) {
	t.Helper()
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("line %q missing prefix %q", line, prefix)
	}
	raw := strings.TrimPrefix(line, prefix)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	if len(decoded) != expectedLen {
		t.Fatalf("decoded len = %d, want %d", len(decoded), expectedLen)
	}
}

func lookupMap(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func assertFileMode600(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %q = %#o, want 0600", path, info.Mode().Perm())
	}
}

func assertFileContainsPrefix(t *testing.T, path, prefix string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	text := strings.TrimSpace(string(data))
	if !strings.HasPrefix(text, prefix) {
		t.Fatalf("file %q value %q missing prefix %q", path, text, prefix)
	}
}

func assertFileBase64Len(t *testing.T, path string, expectedLen int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	raw := strings.TrimSpace(string(data))
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode %q: %v", path, err)
	}
	if len(decoded) != expectedLen {
		t.Fatalf("decoded len from %q = %d, want %d", path, len(decoded), expectedLen)
	}
}
