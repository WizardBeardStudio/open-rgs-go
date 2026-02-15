package evidence

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"testing"
)

func TestResolveValueSourcePriority(t *testing.T) {
	f := t.TempDir() + "/val.txt"
	if err := os.WriteFile(f, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	t.Setenv("VAL_ENV", "from-env")
	t.Setenv("VAL_FILE_ENV", f)
	t.Setenv("VAL_CMD_ENV", "printf from-command")

	got, err := resolveValueSource("VAL_ENV", "VAL_FILE_ENV", "VAL_CMD_ENV")
	if err != nil {
		t.Fatalf("resolveValueSource: %v", err)
	}
	if got != "from-file" {
		t.Fatalf("expected file precedence, got %q", got)
	}

	t.Setenv("VAL_FILE_ENV", "")
	got, err = resolveValueSource("VAL_ENV", "VAL_FILE_ENV", "VAL_CMD_ENV")
	if err != nil {
		t.Fatalf("resolveValueSource command: %v", err)
	}
	if got != "from-command" {
		t.Fatalf("expected command precedence, got %q", got)
	}

	t.Setenv("VAL_CMD_ENV", "")
	got, err = resolveValueSource("VAL_ENV", "VAL_FILE_ENV", "VAL_CMD_ENV")
	if err != nil {
		t.Fatalf("resolveValueSource env: %v", err)
	}
	if got != "from-env" {
		t.Fatalf("expected env fallback, got %q", got)
	}
}

func TestResolveEd25519PublicKeyFromFile(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	path := t.TempDir() + "/pub.b64"
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600); err != nil {
		t.Fatalf("write public key file: %v", err)
	}
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY_FILE", path)
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", DefaultVerifyEvidenceAttestationKeyID)

	got, err := resolveEd25519PublicKey(DefaultVerifyEvidenceAttestationKeyID)
	if err != nil {
		t.Fatalf("resolve public key file: %v", err)
	}
	if len(got) != ed25519.PublicKeySize {
		t.Fatalf("unexpected public key length: %d", len(got))
	}
}

func TestResolveEd25519PublicKeyFromCommand(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY_COMMAND", "printf "+base64.StdEncoding.EncodeToString(pub))
	t.Setenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", DefaultVerifyEvidenceAttestationKeyID)

	got, err := resolveEd25519PublicKey(DefaultVerifyEvidenceAttestationKeyID)
	if err != nil {
		t.Fatalf("resolve public key command: %v", err)
	}
	if len(got) != ed25519.PublicKeySize {
		t.Fatalf("unexpected public key length: %d", len(got))
	}
}
