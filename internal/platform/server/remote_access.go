package server

import (
	"context"
	"database/sql"
	"errors"
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

	trusted              []*net.IPNet
	mu                   sync.Mutex
	logs                 []RemoteAccessActivity
	nextID               int64
	db                   *sql.DB
	disableInMemoryCache bool
	failClosedLogPersist bool
	inMemoryLogCap       int
}

var errRemoteAccessLogCapacityExceeded = errors.New("remote access activity log capacity exceeded")

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

func (g *RemoteAccessGuard) SetDB(db *sql.DB) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.db = db
}

func (g *RemoteAccessGuard) SetDisableInMemoryActivityCache(disable bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.disableInMemoryCache = disable
}

func (g *RemoteAccessGuard) SetFailClosedOnLogPersistenceFailure(enable bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failClosedLogPersist = enable
}

func (g *RemoteAccessGuard) SetInMemoryActivityLogCap(cap int) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if cap < 0 {
		cap = 0
	}
	g.inMemoryLogCap = cap
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
	now := g.now()
	g.mu.Lock()
	g.nextID++
	id := g.nextID
	db := g.db
	g.mu.Unlock()
	res := audit.ResultSuccess
	if outcome != "allowed" {
		res = audit.ResultDenied
	}
	ev := audit.Event{
		AuditID:      "remote-access-" + strconv.FormatInt(id, 10),
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
	}
	if db != nil {
		_ = appendAuditEventToDB(context.Background(), db, ev)
	}
	_, _ = g.AuditStore.Append(ev)
}

func (g *RemoteAccessGuard) logActivity(r *http.Request, sourceIP, sourcePort string, allowed bool, reason string) error {
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
	if !g.disableInMemoryCache {
		if g.db == nil && g.inMemoryLogCap > 0 && len(g.logs) >= g.inMemoryLogCap {
			g.mu.Unlock()
			return errRemoteAccessLogCapacityExceeded
		}
		g.logs = append(g.logs, entry)
	}
	db := g.db
	g.mu.Unlock()
	if db != nil {
		if err := g.persistActivity(context.Background(), db, entry); err != nil {
			return err
		}
	}
	return nil
}

func (g *RemoteAccessGuard) Activities() []RemoteAccessActivity {
	g.mu.Lock()
	db := g.db
	disableInMemory := g.disableInMemoryCache
	out := make([]RemoteAccessActivity, len(g.logs))
	copy(out, g.logs)
	g.mu.Unlock()
	if db != nil {
		dbOut, err := g.activitiesFromDB(context.Background(), db)
		if err == nil {
			return dbOut
		}
	}
	if disableInMemory {
		return nil
	}
	return out
}

func (g *RemoteAccessGuard) persistActivity(ctx context.Context, db *sql.DB, activity RemoteAccessActivity) error {
	const q = `
INSERT INTO remote_access_activity (
  occurred_at, source_ip, source_port, destination_host, destination_port, path, method, allowed, reason
)
VALUES (
  $1::timestamptz, $2, $3, $4, $5, $6, $7, $8, $9
)
`
	_, err := db.ExecContext(ctx, q,
		activity.Timestamp,
		activity.SourceIP,
		activity.SourcePort,
		activity.Destination,
		activity.DestinationPort,
		activity.Path,
		activity.Method,
		activity.Allowed,
		activity.Reason,
	)
	return err
}

func (g *RemoteAccessGuard) activitiesFromDB(ctx context.Context, db *sql.DB) ([]RemoteAccessActivity, error) {
	const q = `
SELECT occurred_at, source_ip, source_port, destination_host, destination_port, path, method, allowed, reason
FROM remote_access_activity
ORDER BY occurred_at DESC, activity_id DESC
LIMIT 5000
`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RemoteAccessActivity, 0)
	for rows.Next() {
		var (
			occurredAt time.Time
			entry      RemoteAccessActivity
		)
		if err := rows.Scan(
			&occurredAt,
			&entry.SourceIP,
			&entry.SourcePort,
			&entry.Destination,
			&entry.DestinationPort,
			&entry.Path,
			&entry.Method,
			&entry.Allowed,
			&entry.Reason,
		); err != nil {
			return nil, err
		}
		entry.Timestamp = occurredAt.UTC().Format(time.RFC3339Nano)
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (g *RemoteAccessGuard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.isAdminPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		sourceIP, sourcePort := g.extractSourceIP(r)
		if !g.isTrusted(sourceIP) {
			if err := g.logActivity(r, sourceIP, sourcePort, false, "source ip outside trusted network"); err != nil {
				g.mu.Lock()
				failClosed := g.failClosedLogPersist
				g.mu.Unlock()
				if failClosed {
					http.Error(w, "remote access logging unavailable", http.StatusServiceUnavailable)
					return
				}
			}
			g.appendAudit(r.URL.Path, sourceIP, "denied", "source ip outside trusted network")
			http.Error(w, "remote access denied", http.StatusForbidden)
			return
		}

		if err := g.logActivity(r, sourceIP, sourcePort, true, ""); err != nil {
			g.mu.Lock()
			failClosed := g.failClosedLogPersist
			g.mu.Unlock()
			if failClosed {
				http.Error(w, "remote access logging unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		g.appendAudit(r.URL.Path, sourceIP, "allowed", "")
		next.ServeHTTP(w, r)
	})
}
