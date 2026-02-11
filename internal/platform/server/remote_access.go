package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
)

type RemoteAccessActivity struct {
	Timestamp       string
	SourceIP        string
	SourcePort      string
	Destination     string
	DestinationPort string
	Path            string
	Method          string
	Allowed         bool
	Reason          string
}

type RemoteAccessGuard struct {
	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	trusted []*net.IPNet
	mu      sync.Mutex
	logs    []RemoteAccessActivity
	nextID  int64
}

func NewRemoteAccessGuard(clk clock.Clock, store *audit.InMemoryStore, cidrs []string) (*RemoteAccessGuard, error) {
	trusted := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted cidr %q: %w", c, err)
		}
		trusted = append(trusted, ipnet)
	}
	if len(trusted) == 0 {
		for _, c := range []string{"127.0.0.1/32", "::1/128"} {
			_, ipnet, _ := net.ParseCIDR(c)
			trusted = append(trusted, ipnet)
		}
	}
	return &RemoteAccessGuard{Clock: clk, AuditStore: store, trusted: trusted}, nil
}

func (g *RemoteAccessGuard) now() time.Time {
	if g.Clock == nil {
		return time.Now().UTC()
	}
	return g.Clock.Now().UTC()
}

func (g *RemoteAccessGuard) isAdminPath(path string) bool {
	return strings.HasPrefix(path, "/v1/config") || strings.HasPrefix(path, "/v1/reporting") || strings.HasPrefix(path, "/v1/audit")
}

func (g *RemoteAccessGuard) extractSourceIP(r *http.Request) (string, string) {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		return ip, ""
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host, port
	}
	return strings.TrimSpace(r.RemoteAddr), ""
}

func (g *RemoteAccessGuard) isTrusted(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range g.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (g *RemoteAccessGuard) appendAudit(path, sourceIP, outcome, reason string) {
	if g.AuditStore == nil {
		return
	}
	g.nextID++
	now := g.now()
	res := audit.ResultSuccess
	if outcome != "allowed" {
		res = audit.ResultDenied
	}
	_, _ = g.AuditStore.Append(audit.Event{
		AuditID:      "remote-access-" + strconv.FormatInt(g.nextID, 10),
		OccurredAt:   now,
		RecordedAt:   now,
		ActorID:      sourceIP,
		ActorType:    "remote",
		AuthContext:  "path=" + path,
		ObjectType:   "remote_access",
		ObjectID:     path,
		Action:       outcome,
		Before:       []byte(`{}`),
		After:        []byte(`{}`),
		Result:       res,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	})
}

func (g *RemoteAccessGuard) logActivity(r *http.Request, sourceIP, sourcePort string, allowed bool, reason string) {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
		port = ""
	}
	entry := RemoteAccessActivity{
		Timestamp:       g.now().Format(time.RFC3339Nano),
		SourceIP:        sourceIP,
		SourcePort:      sourcePort,
		Destination:     host,
		DestinationPort: port,
		Path:            r.URL.Path,
		Method:          r.Method,
		Allowed:         allowed,
		Reason:          reason,
	}
	g.mu.Lock()
	g.logs = append(g.logs, entry)
	g.mu.Unlock()
}

func (g *RemoteAccessGuard) Activities() []RemoteAccessActivity {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]RemoteAccessActivity, len(g.logs))
	copy(out, g.logs)
	return out
}

func (g *RemoteAccessGuard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		sourceIP, sourcePort := g.extractSourceIP(r)
		if !g.isTrusted(sourceIP) {
			g.logActivity(r, sourceIP, sourcePort, false, "source ip outside trusted network")
			g.appendAudit(r.URL.Path, sourceIP, "denied", "source ip outside trusted network")
			http.Error(w, "remote access denied", http.StatusForbidden)
			return
		}

		g.logActivity(r, sourceIP, sourcePort, true, "")
		g.appendAudit(r.URL.Path, sourceIP, "allowed", "")
		next.ServeHTTP(w, r)
	})
}
