package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
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
	cfg := parseConfig()

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

	if cfg.emitKeyID {
		fmt.Printf("%s=%s\n", cfg.keyIDVar, cfg.keyID)
	}
	fmt.Printf("%s=%s\n", cfg.privateVar, privateValue)
	fmt.Printf("%s=%s\n", cfg.publicVar, publicValue)
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.alg, "alg", envOr("RGS_ATTEST_KEYGEN_ALG", algEd25519), "attestation algorithm")
	flag.StringVar(&cfg.keyID, "key-id", envOr2("RGS_ATTEST_KEYGEN_KEY_ID", "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID", "ci-active"), "attestation key id")
	flag.StringVar(&cfg.keyIDVar, "key-id-var", envOr("RGS_ATTEST_KEYGEN_KEY_ID_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID"), "env var name used when emitting key id")
	flag.BoolVar(&cfg.emitKeyID, "emit-key-id", envBoolOr("RGS_ATTEST_KEYGEN_EMIT_KEY_ID", true), "emit key id env assignment")
	flag.StringVar(&cfg.privateVar, "private-var", envOr("RGS_ATTEST_KEYGEN_PRIVATE_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY"), "env var name for private key material")
	flag.StringVar(&cfg.publicVar, "public-var", envOr("RGS_ATTEST_KEYGEN_PUBLIC_VAR", "RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS"), "env var name for public key material")
	flag.BoolVar(&cfg.ringOutput, "ring", envBoolOr("RGS_ATTEST_KEYGEN_RING", true), "emit key_id:value format")
	flag.StringVar(&cfg.privateMaterialFmt, "private-material", envOr("RGS_ATTEST_KEYGEN_PRIVATE_MATERIAL", "seed"), "private material format: seed or private")
	flag.Parse()
	return cfg
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envOr2(primary, secondary, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(primary)); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(secondary)); v != "" {
		return v
	}
	return fallback
}

func envBoolOr(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
