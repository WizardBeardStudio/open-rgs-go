package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateProductionRuntimeStrictRequirements(t *testing.T) {
	cases := []struct {
		name          string
		strict        bool
		databaseURL   string
		tlsEnabled    bool
		jwtSecret     string
		jwtKeysetSpec string
		jwtKeysetFile string
		wantErr       bool
	}{
		{
			name:          "non-strict allows dev defaults",
			strict:        false,
			databaseURL:   "",
			tlsEnabled:    false,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "",
			wantErr:       false,
		},
		{
			name:          "strict requires database",
			strict:        true,
			databaseURL:   "",
			tlsEnabled:    true,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict requires tls",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    false,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict rejects default jwt secret without keyset",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "",
			wantErr:       true,
		},
		{
			name:          "strict allows keyset with default single secret value",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "k1:rotated-secret",
			wantErr:       false,
		},
		{
			name:          "strict valid config",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "prod-secret",
			jwtKeysetSpec: "",
			wantErr:       false,
		},
		{
			name:          "strict allows keyset file with default secret",
			strict:        true,
			databaseURL:   "postgres://x",
			tlsEnabled:    true,
			jwtSecret:     "dev-insecure-change-me",
			jwtKeysetSpec: "",
			jwtKeysetFile: "/etc/rgs/jwt-keyset.json",
			wantErr:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProductionRuntime(tc.strict, tc.databaseURL, tc.tlsEnabled, tc.jwtSecret, tc.jwtKeysetSpec, tc.jwtKeysetFile)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateProductionRuntime() err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestLoadJWTKeysetFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-keyset.json")
	if err := os.WriteFile(path, []byte(`{"active_kid":"k2","keys":{"k1":"secret1","k2":"secret2"}}`), 0o600); err != nil {
		t.Fatalf("write keyset: %v", err)
	}
	keyset, fingerprint, err := loadJWTKeyset("ignored", "", "default", path)
	if err != nil {
		t.Fatalf("load keyset: %v", err)
	}
	if keyset.ActiveKID != "k2" {
		t.Fatalf("expected active kid k2, got=%s", keyset.ActiveKID)
	}
	if fingerprint == "" {
		t.Fatalf("expected non-empty fingerprint")
	}
}

func TestParseKeyValueSecrets(t *testing.T) {
	keys := parseKeyValueSecrets("k1:secret1, k2:secret2, invalid, :missing")
	if len(keys) != 2 {
		t.Fatalf("expected 2 parsed keys, got=%d", len(keys))
	}
	if string(keys["k1"]) != "secret1" || string(keys["k2"]) != "secret2" {
		t.Fatalf("unexpected key parsing result")
	}
}
