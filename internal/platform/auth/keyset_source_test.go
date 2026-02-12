package auth

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadHMACKeysetFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "jwt-keyset.json")
	if err := os.WriteFile(path, []byte(`{"active_kid":"k2","keys":{"k1":"secret1","k2":"secret2"}}`), 0o600); err != nil {
		t.Fatalf("write keyset file: %v", err)
	}

	keyset, err := LoadHMACKeysetFile(path)
	if err != nil {
		t.Fatalf("load keyset file: %v", err)
	}
	if keyset.ActiveKID != "k2" {
		t.Fatalf("expected active kid k2, got=%q", keyset.ActiveKID)
	}
	if string(keyset.Keys["k1"]) != "secret1" || string(keyset.Keys["k2"]) != "secret2" {
		t.Fatalf("unexpected key material")
	}
}

func TestLoadHMACKeysetJSON(t *testing.T) {
	keyset, err := LoadHMACKeysetJSON([]byte(`{"active_kid":"k1","keys":{"k1":"secret1"}}`))
	if err != nil {
		t.Fatalf("load keyset json: %v", err)
	}
	if keyset.ActiveKID != "k1" {
		t.Fatalf("expected active kid k1, got=%q", keyset.ActiveKID)
	}
}

func TestLoadHMACKeysetCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command format differs on windows")
	}
	keyset, err := LoadHMACKeysetCommand(context.Background(), `printf '{"active_kid":"k1","keys":{"k1":"secret1"}}'`)
	if err != nil {
		t.Fatalf("load keyset command: %v", err)
	}
	if keyset.ActiveKID != "k1" {
		t.Fatalf("expected active kid k1, got=%q", keyset.ActiveKID)
	}
}
