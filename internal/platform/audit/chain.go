package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func ComputeHash(prev string, e Event) string {
	h := sha256.New()
	_, _ = h.Write([]byte(prev))
	_, _ = h.Write([]byte("|" + e.AuditID))
	_, _ = h.Write([]byte("|" + e.RecordedAt.UTC().Format("2006-01-02T15:04:05.999999999Z")))
	_, _ = h.Write([]byte("|" + e.ActorID + "|" + e.Action + "|" + string(e.Result)))
	_, _ = h.Write([]byte(fmt.Sprintf("|%x|%x", e.Before, e.After)))
	return hex.EncodeToString(h.Sum(nil))
}
