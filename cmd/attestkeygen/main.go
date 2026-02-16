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
	"strings"
)

const (
	algEd25519 = "ed25519"
)

type config struct {
	alg                string
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
	if cfg.privateMaterialFmt != "seed" && cfg.privateMaterialFmt != "private" {
		fmt.Fprintf(os.Stderr, "unsupported private material format: %s (expected seed or private)\n", cfg.privateMaterialFmt)
		os.Exit(2)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate ed25519 keypair: %v\n", err)
		os.Exit(1)
	}

	writeAssignments(os.Stdout, renderAssignments(cfg, pub, priv))
}

func parseConfig(args []string, lookup func(string) string) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("attestkeygen", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.alg, "alg", envOr(lookup, "RGS_ATTEST_KEYGEN_ALG", algEd25519), "attestation algorithm")
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

func renderAssignments(cfg config, pub ed25519.PublicKey, priv ed25519.PrivateKey) []string {
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
	lines := make([]string, 0, 3)
	if cfg.emitKeyID {
		lines = append(lines, fmt.Sprintf("%s=%s", cfg.keyIDVar, cfg.keyID))
	}
	lines = append(lines, fmt.Sprintf("%s=%s", cfg.privateVar, privateValue))
	lines = append(lines, fmt.Sprintf("%s=%s", cfg.publicVar, publicValue))
	return lines
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
