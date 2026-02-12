package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type hmacKeysetFile struct {
	ActiveKID string            `json:"active_kid"`
	Keys      map[string]string `json:"keys"`
}

func LoadHMACKeysetFile(path string) (HMACKeyset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return HMACKeyset{}, fmt.Errorf("read jwt keyset file: %w", err)
	}
	var f hmacKeysetFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return HMACKeyset{}, fmt.Errorf("decode jwt keyset file: %w", err)
	}
	active := strings.TrimSpace(f.ActiveKID)
	if active == "" {
		active = "default"
	}
	keys := make(map[string][]byte, len(f.Keys))
	for kid, secret := range f.Keys {
		kid = strings.TrimSpace(kid)
		secret = strings.TrimSpace(secret)
		if kid == "" || secret == "" {
			continue
		}
		keys[kid] = []byte(secret)
	}
	if len(keys) == 0 {
		return HMACKeyset{}, fmt.Errorf("jwt keyset file contains no keys")
	}
	if _, ok := keys[active]; !ok {
		return HMACKeyset{}, fmt.Errorf("active kid %q not found in keyset file", active)
	}
	return HMACKeyset{
		ActiveKID: active,
		Keys:      keys,
	}, nil
}
