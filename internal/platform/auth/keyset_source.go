package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
	return LoadHMACKeysetJSON(raw)
}

func LoadHMACKeysetCommand(ctx context.Context, command string) (HMACKeyset, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return HMACKeyset{}, fmt.Errorf("jwt keyset command is empty")
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}
	out, err := cmd.Output()
	if err != nil {
		return HMACKeyset{}, fmt.Errorf("execute jwt keyset command: %w", err)
	}
	return LoadHMACKeysetJSON(out)
}

func LoadHMACKeysetJSON(raw []byte) (HMACKeyset, error) {
	var f hmacKeysetFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return HMACKeyset{}, fmt.Errorf("decode jwt keyset payload: %w", err)
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
		return HMACKeyset{}, fmt.Errorf("jwt keyset payload contains no keys")
	}
	if _, ok := keys[active]; !ok {
		return HMACKeyset{}, fmt.Errorf("active kid %q not found in keyset payload", active)
	}
	return HMACKeyset{
		ActiveKID: active,
		Keys:      keys,
	}, nil
}
