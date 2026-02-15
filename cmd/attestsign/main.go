package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const (
	defaultKeyID          = "dev-default"
	algEd25519            = "ed25519"
	defaultDevSeedContext = "open-rgs-go-dev-attestation-seed"
)

func main() {
	in := flag.String("in", "", "input attestation file")
	out := flag.String("out", "", "output signature file")
	alg := flag.String("alg", algEd25519, "signature algorithm: ed25519")
	keyID := flag.String("key-id", defaultKeyID, "attestation key id")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/attestsign --in <attestation.json> --out <attestation.sig> [--alg ed25519] [--key-id <id>]")
		os.Exit(2)
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read attestation: %v\n", err)
		os.Exit(1)
	}

	var sigHex string
	switch *alg {
	case algEd25519:
		priv, err := resolveEd25519PrivateKey(*keyID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve ed25519 private key: %v\n", err)
			os.Exit(1)
		}
		sig := ed25519.Sign(priv, data)
		sigHex = hex.EncodeToString(sig)
	default:
		fmt.Fprintf(os.Stderr, "unsupported algorithm: %s\n", *alg)
		os.Exit(1)
	}

	if err := os.WriteFile(*out, []byte(sigHex+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write signature: %v\n", err)
		os.Exit(1)
	}
}

func resolveEd25519PrivateKey(keyID string) (ed25519.PrivateKey, error) {
	enforce := os.Getenv("RGS_VERIFY_EVIDENCE_ENFORCE_ATTESTATION_KEY") == "true" || os.Getenv("GITHUB_ACTIONS") == "true"
	allowInline := os.Getenv("RGS_VERIFY_EVIDENCE_ALLOW_INLINE_PRIVATE_KEY") == "true"
	if enforce && !allowInline {
		if strings.TrimSpace(os.Getenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY")) != "" || strings.TrimSpace(os.Getenv("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS")) != "" {
			return nil, fmt.Errorf("inline ed25519 private-key env vars are disabled in strict/CI mode")
		}
	}

	keyRingRaw, err := resolveValueSource(
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS_FILE",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS_COMMAND",
	)
	if err != nil {
		return nil, err
	}
	if keyRingRaw != "" {
		keyRing := map[string]string{}
		for _, part := range strings.Split(keyRingRaw, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			idx := strings.IndexByte(p, ':')
			if idx <= 0 || idx >= len(p)-1 {
				return nil, fmt.Errorf("invalid RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS entry: %q", p)
			}
			id := strings.TrimSpace(p[:idx])
			val := strings.TrimSpace(p[idx+1:])
			if id == "" || val == "" {
				return nil, fmt.Errorf("invalid RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS entry: %q", p)
			}
			keyRing[id] = val
		}
		raw, ok := keyRing[keyID]
		if !ok {
			ids := make([]string, 0, len(keyRing))
			for id := range keyRing {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			return nil, fmt.Errorf("no ed25519 private key for key_id=%q (available: %s)", keyID, strings.Join(ids, ","))
		}
		return parseEd25519PrivateKey(raw)
	}

	raw, err := resolveValueSource(
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY_FILE",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY_COMMAND",
	)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		if !enforce && keyID == defaultKeyID {
			return defaultDevEd25519PrivateKey(), nil
		}
		return nil, fmt.Errorf("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY or RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS is required for ed25519")
	}
	singleID := os.Getenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID")
	if singleID == "" {
		singleID = defaultKeyID
	}
	if singleID != keyID {
		return nil, fmt.Errorf("ed25519 key_id mismatch: requested=%q configured=%q", keyID, singleID)
	}
	return parseEd25519PrivateKey(raw)
}

func resolveValueSource(valueEnv, fileEnv, commandEnv string) (string, error) {
	if p := strings.TrimSpace(os.Getenv(fileEnv)); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read %s file %q: %w", valueEnv, p, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if cmdRaw := strings.TrimSpace(os.Getenv(commandEnv)); cmdRaw != "" {
		out, err := exec.Command("bash", "-lc", cmdRaw).Output()
		if err != nil {
			return "", fmt.Errorf("run %s command: %w", valueEnv, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if v := strings.TrimSpace(os.Getenv(valueEnv)); v != "" {
		return v, nil
	}
	return "", nil
}

func parseEd25519PrivateKey(raw string) (ed25519.PrivateKey, error) {
	decoded, err := base64.StdEncoding.DecodeString(normalizeKeyMaterial(raw))
	if err != nil {
		return nil, fmt.Errorf("decode base64 private key: %w", err)
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("invalid ed25519 private key length: %d", len(decoded))
	}
}

func defaultDevEd25519PrivateKey() ed25519.PrivateKey {
	sum := sha256.Sum256([]byte(defaultDevSeedContext))
	return ed25519.NewKeyFromSeed(sum[:ed25519.SeedSize])
}

func normalizeKeyMaterial(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) >= 2 {
		first := trimmed[0]
		last := trimmed[len(trimmed)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}
	}
	return trimmed
}
