package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	algEd25519 = "ed25519"
)

type config struct {
	alg                string
	format             string
	outDir             string
	keyID              string
	keyIDVar           string
	emitKeyID          bool
	privateVar         string
	publicVar          string
	ringOutput         bool
	privateMaterialFmt string
}

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse config: %v\n", err)
		os.Exit(2)
	}

	if cfg.alg != algEd25519 {
		fmt.Fprintf(os.Stderr, "unsupported algorithm: %s\n", cfg.alg)
		os.Exit(2)
	}
	if strings.TrimSpace(cfg.keyID) == "" {
		fmt.Fprintln(os.Stderr, "key id must be non-empty")
		os.Exit(2)
	}
	if cfg.format != "assignments" && cfg.format != "github-secrets" {
		fmt.Fprintf(os.Stderr, "unsupported format: %s (expected assignments or github-secrets)\n", cfg.format)
		os.Exit(2)
	}
	if cfg.privateMaterialFmt != "seed" && cfg.privateMaterialFmt != "private" {
		fmt.Fprintf(os.Stderr, "unsupported private material format: %s (expected seed or private)\n", cfg.privateMaterialFmt)
		os.Exit(2)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate ed25519 keypair: %v\n", err)
		os.Exit(1)
	}

	material := renderKeyMaterial(cfg, pub, priv)
	if cfg.outDir != "" {
		lines, err := writeSecretFiles(cfg, material)
		if err != nil {
			fmt.Fprintf(os.Stderr, "write key files: %v\n", err)
			os.Exit(1)
		}
		writeAssignments(os.Stdout, lines)
		return
	}
	writeAssignments(os.Stdout, renderAssignments(cfg, material))
}

func parseConfig(args []string, lookup func(string) string) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("attestkeygen", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.alg, "alg", envOr(lookup, "RGS_ATTEST_KEYGEN_ALG", algEd25519), "attestation algorithm")
	flags.StringVar(&cfg.format, "format", envOr(lookup, "RGS_ATTEST_KEYGEN_FORMAT", "assignments"), "output format: assignments or github-secrets")
	flags.StringVar(&cfg.outDir, "out-dir", envOr(lookup, "RGS_ATTEST_KEYGEN_OUT_DIR", ""), "directory to write generated key material files")
	flags.StringVar(&cfg.keyID, "key-id", envOr2(lookup, "RGS_ATTEST_KEYGEN_KEY_ID", "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", "ci-active"), "attestation key id")
	flags.StringVar(&cfg.keyIDVar, "key-id-var", envOr(lookup, "RGS_ATTEST_KEYGEN_KEY_ID_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID"), "env var name used when emitting key id")
	flags.BoolVar(&cfg.emitKeyID, "emit-key-id", envBoolOr(lookup, "RGS_ATTEST_KEYGEN_EMIT_KEY_ID", true), "emit key id env assignment")
	flags.StringVar(&cfg.privateVar, "private-var", envOr(lookup, "RGS_ATTEST_KEYGEN_PRIVATE_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY"), "env var name for private key material")
	flags.StringVar(&cfg.publicVar, "public-var", envOr(lookup, "RGS_ATTEST_KEYGEN_PUBLIC_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS"), "env var name for public key material")
	flags.BoolVar(&cfg.ringOutput, "ring", envBoolOr(lookup, "RGS_ATTEST_KEYGEN_RING", true), "emit key_id:value format")
	flags.StringVar(&cfg.privateMaterialFmt, "private-material", envOr(lookup, "RGS_ATTEST_KEYGEN_PRIVATE_MATERIAL", "seed"), "private material format: seed or private")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cfg, err
		}
		return cfg, err
	}
	return cfg, nil
}

func renderKeyMaterial(cfg config, pub ed25519.PublicKey, priv ed25519.PrivateKey) keyMaterial {
	privateBytes := []byte(priv)
	if cfg.privateMaterialFmt == "seed" {
		privateBytes = priv.Seed()
	}
	privateValue := base64.StdEncoding.EncodeToString(privateBytes)
	publicValue := base64.StdEncoding.EncodeToString(pub)
	if cfg.ringOutput {
		privateValue = cfg.keyID + ":" + privateValue
		publicValue = cfg.keyID + ":" + publicValue
	}
	return keyMaterial{
		privateValue: privateValue,
		publicValue:  publicValue,
	}
}

type keyMaterial struct {
	privateValue string
	publicValue  string
}

func renderAssignments(cfg config, material keyMaterial) []string {
	if cfg.format == "github-secrets" {
		return []string{material.privateValue, material.publicValue}
	}
	lines := make([]string, 0, 3)
	if cfg.emitKeyID {
		lines = append(lines, fmt.Sprintf("%s=%s", cfg.keyIDVar, cfg.keyID))
	}
	lines = append(lines, fmt.Sprintf("%s=%s", cfg.privateVar, material.privateValue))
	lines = append(lines, fmt.Sprintf("%s=%s", cfg.publicVar, material.publicValue))
	return lines
}

func writeSecretFiles(cfg config, material keyMaterial) ([]string, error) {
	if err := os.MkdirAll(cfg.outDir, 0o700); err != nil {
		return nil, fmt.Errorf("create out dir: %w", err)
	}

	privateFile := "private-key.txt"
	publicFile := "public-key.txt"
	privateFileVar := "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY_FILE"
	publicFileVar := "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY_FILE"
	if cfg.ringOutput {
		privateFile = "private-keys.txt"
		publicFile = "public-keys.txt"
		privateFileVar = "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS_FILE"
		publicFileVar = "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS_FILE"
	}

	privatePath := filepath.Join(cfg.outDir, privateFile)
	publicPath := filepath.Join(cfg.outDir, publicFile)
	if err := os.WriteFile(privatePath, []byte(material.privateValue+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write private key file: %w", err)
	}
	if err := os.WriteFile(publicPath, []byte(material.publicValue+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write public key file: %w", err)
	}

	lines := make([]string, 0, 3)
	lines = append(lines, fmt.Sprintf("%s=%s", cfg.keyIDVar, cfg.keyID))
	lines = append(lines, fmt.Sprintf("%s=%s", privateFileVar, privatePath))
	lines = append(lines, fmt.Sprintf("%s=%s", publicFileVar, publicPath))
	return lines, nil
}

func writeAssignments(out io.Writer, lines []string) {
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
}

func envOr(lookup func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(lookup(key)); v != "" {
		return v
	}
	return fallback
}

func envOr2(lookup func(string) string, primary, secondary, fallback string) string {
	if v := strings.TrimSpace(lookup(primary)); v != "" {
		return v
	}
	if v := strings.TrimSpace(lookup(secondary)); v != "" {
		return v
	}
	return fallback
}

func envBoolOr(lookup func(string) string, key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(lookup(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
