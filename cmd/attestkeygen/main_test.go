package main

import (
	"crypto/ed25519"
	"encoding/base64"
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
	lines := renderAssignments(cfg, pub, priv)
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
	lines := renderAssignments(cfg, pub, priv)
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
	lines := renderAssignments(cfg, pub, priv)
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if strings.HasPrefix(lines[0], "PRIVATE=") || strings.HasPrefix(lines[1], "PUBLIC=") || strings.HasPrefix(lines[0], "KEY_ID=") {
		t.Fatalf("github-secrets format should not contain assignments: %#v", lines)
	}
	assertValueLen(t, lines[0], "ci-active:", ed25519.SeedSize)
	assertValueLen(t, lines[1], "ci-active:", ed25519.PublicKeySize)
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
