package main

import (
	"crypto/ed25519"
	"crypto/hmac"
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
	defaultKeyID  = "dev-default"
	defaultHMAC   = "open-rgs-go-dev-attestation-key"
	algHMACSHA256 = "hmac-sha256"
	algEd25519    = "ed25519"
)

func main() {
	in := flag.String("in", "", "input attestation file")
	out := flag.String("out", "", "output signature file")
	alg := flag.String("alg", algHMACSHA256, "signature algorithm: hmac-sha256|ed25519")
	keyID := flag.String("key-id", defaultKeyID, "attestation key id")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/attestsign --in <attestation.json> --out <attestation.sig> [--alg hmac-sha256|ed25519] [--key-id <id>]")
		os.Exit(2)
	}

	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read attestation: %v\n", err)
		os.Exit(1)
	}

	var sigHex string
	switch *alg {
	case algHMACSHA256:
		key, err := resolveHMACKey(*keyID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve hmac key: %v\n", err)
			os.Exit(1)
		}
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(data)
		sigHex = hex.EncodeToString(mac.Sum(nil))
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

func resolveHMACKey(keyID string) (string, error) {
	keyRingRaw, err := resolveValueSource(
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS_FILE",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS_COMMAND",
	)
	if err != nil {
		return "", err
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
				return "", fmt.Errorf("invalid RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS entry: %q", p)
			}
			id := strings.TrimSpace(p[:idx])
			val := strings.TrimSpace(p[idx+1:])
			if id == "" || val == "" {
				return "", fmt.Errorf("invalid RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS entry: %q", p)
			}
			keyRing[id] = val
		}
		if key, ok := keyRing[keyID]; ok {
			return key, nil
		}
		ids := make([]string, 0, len(keyRing))
		for id := range keyRing {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return "", fmt.Errorf("no hmac key for key_id=%q in RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS (available: %s)", keyID, strings.Join(ids, ","))
	}

	key, err := resolveValueSource(
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEY",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_FILE",
		"RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_COMMAND",
	)
	if err != nil {
		return "", err
	}
	if key == "" {
		if keyID != defaultKeyID {
			return "", fmt.Errorf("no hmac key available for key_id=%q", keyID)
		}
		return defaultHMAC, nil
	}
	singleID := os.Getenv("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID")
	if singleID == "" {
		singleID = defaultKeyID
	}
	if singleID != keyID {
		return "", fmt.Errorf("hmac key_id mismatch: requested=%q configured=%q", keyID, singleID)
	}
	return key, nil
}

func resolveEd25519PrivateKey(keyID string) (ed25519.PrivateKey, error) {
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
	if v := strings.TrimSpace(os.Getenv(valueEnv)); v != "" {
		return v, nil
	}
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
	return "", nil
}

func parseEd25519PrivateKey(raw string) (ed25519.PrivateKey, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
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
