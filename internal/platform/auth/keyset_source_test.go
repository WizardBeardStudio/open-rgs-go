package auth

import (
	"os"
	"path/filepath"
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
