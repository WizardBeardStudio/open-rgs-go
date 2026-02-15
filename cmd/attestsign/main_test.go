package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestResolveValueSourcePriority(t *testing.T) {
	f := t.TempDir() + "/val.txt"
	t.Setenv("TEST_VAL", "from-env")
	t.Setenv("TEST_VAL_FILE", f)
	t.Setenv("TEST_VAL_COMMAND", "printf from-command")
	if err := osWriteFile(f, []byte("from-file\n")); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := resolveValueSource("TEST_VAL", "TEST_VAL_FILE", "TEST_VAL_COMMAND")
	if err != nil {
		t.Fatalf("resolveValueSource: %v", err)
	}
	if got != "from-file" {
		t.Fatalf("expected file precedence, got %q", got)
	}

	t.Setenv("TEST_VAL_FILE", "")
	got, err = resolveValueSource("TEST_VAL", "TEST_VAL_FILE", "TEST_VAL_COMMAND")
	if err != nil {
		t.Fatalf("resolveValueSource command: %v", err)
	}
	if got != "from-command" {
		t.Fatalf("expected command precedence, got %q", got)
	}

	t.Setenv("TEST_VAL_COMMAND", "")
	got, err = resolveValueSource("TEST_VAL", "TEST_VAL_FILE", "TEST_VAL_COMMAND")
	if err != nil {
		t.Fatalf("resolveValueSource env: %v", err)
	}
	if got != "from-env" {
		t.Fatalf("expected env fallback, got %q", got)
	}
}

func TestResolveValueSourceFileError(t *testing.T) {
	t.Setenv("ERR_VAL_FILE", "/nonexistent/path/value.txt")
	_, err := resolveValueSource("ERR_VAL", "ERR_VAL_FILE", "ERR_VAL_COMMAND")
	if err == nil {
		t.Fatalf("expected error for missing file source")
	}
}

func TestResolveEd25519PrivateKeyStrictRejectsInline(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	t.Setenv("RGS_VERIFY_EVIDENCE_ENFORCE_ATTESTATION_KEY", "true")
	t.Setenv("RGS_VERIFY_EVIDENCE_ALLOW_INLINE_PRIVATE_KEY", "false")
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY", base64.StdEncoding.EncodeToString(priv))
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", defaultKeyID)

	_, err := resolveEd25519PrivateKey(defaultKeyID)
	if err == nil || !strings.Contains(err.Error(), "inline ed25519 private-key env vars are disabled") {
		t.Fatalf("expected strict inline rejection, got err=%v", err)
	}
}

func TestResolveEd25519PrivateKeyFromFile(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	path := t.TempDir() + "/priv.b64"
	if err := osWriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv))); err != nil {
		t.Fatalf("write private key file: %v", err)
	}
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY_FILE", path)
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", defaultKeyID)

	got, err := resolveEd25519PrivateKey(defaultKeyID)
	if err != nil {
		t.Fatalf("resolve from file: %v", err)
	}
	if len(got) != ed25519.PrivateKeySize {
		t.Fatalf("unexpected key length: %d", len(got))
	}
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
