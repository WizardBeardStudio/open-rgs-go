#!/usr/bin/env bash
set -euo pipefail

key_id="${1:-ci-active}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

cat >"${tmpdir}/main.go" <<'EOF'
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	keyID := "ci-active"
	if len(os.Args) > 1 && os.Args[1] != "" {
		keyID = os.Args[1]
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate ed25519 keypair: %v\n", err)
		os.Exit(1)
	}

	seed := priv.Seed()
	fmt.Printf("RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID=%s\n", keyID)
	fmt.Printf("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY=%s:%s\n", keyID, base64.StdEncoding.EncodeToString(seed))
	fmt.Printf("RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS=%s:%s\n", keyID, base64.StdEncoding.EncodeToString(pub))
}
EOF

go run "${tmpdir}/main.go" "${key_id}"
