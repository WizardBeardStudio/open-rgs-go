package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

type Metrics struct {
	cleanupRunsTotal        *prometheus.CounterVec
	cleanupDeletedTotal     prometheus.Counter
	cleanupLastDeleted      prometheus.Gauge
	cleanupLastRunUnix      prometheus.Gauge
	idempotencyKeysTotal    prometheus.Gauge
	idempotencyKeysExpired  prometheus.Gauge
	loginAttemptsTotal      *prometheus.CounterVec
	lockoutActivations      *prometheus.CounterVec
	identitySessionsActive  prometheus.Gauge
	identitySessionsRevoked prometheus.Gauge
	identitySessionsExpired prometheus.Gauge
}

func NewMetrics() *Metrics {
	return &Metrics{
		cleanupRunsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "cleanup_runs_total",
				Help:      "Total cleanup runs partitioned by result.",
			},
			[]string{"result"},
		),
		cleanupDeletedTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "cleanup_deleted_total",
				Help:      "Total number of expired idempotency keys deleted.",
			},
		),
		cleanupLastDeleted: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "cleanup_last_deleted",
				Help:      "Number of keys deleted in the most recent cleanup run.",
			},
		),
		cleanupLastRunUnix: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "cleanup_last_run_unix",
				Help:      "Unix time of the most recent cleanup run.",
			},
		),
		idempotencyKeysTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "keys_total",
				Help:      "Current count of all idempotency keys.",
			},
		),
		idempotencyKeysExpired: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "ledger_idempotency",
				Name:      "keys_expired",
				Help:      "Current count of expired idempotency keys.",
			},
		),
		loginAttemptsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "identity",
				Name:      "login_attempts_total",
				Help:      "Total identity login attempts by result and actor type.",
			},
			[]string{"result", "actor_type"},
		),
		lockoutActivations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "identity",
				Name:      "lockout_activations_total",
				Help:      "Total identity lockout activations by actor type.",
			},
			[]string{"actor_type"},
		),
		identitySessionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "identity",
				Name:      "sessions_active",
				Help:      "Current count of active identity sessions.",
			},
		),
		identitySessionsRevoked: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "identity",
				Name:      "sessions_revoked",
				Help:      "Current count of revoked identity sessions.",
			},
		),
		identitySessionsExpired: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "open_rgs",
				Subsystem: "identity",
				Name:      "sessions_expired",
				Help:      "Current count of expired identity sessions.",
			},
		),
	}
}

func (m *Metrics) ObserveLedgerIdempotencyCleanup(deleted int64, err error) {
	if m == nil {
		return
	}
	m.cleanupLastRunUnix.Set(float64(time.Now().UTC().Unix()))
	m.cleanupLastDeleted.Set(float64(deleted))
	if err != nil {
		m.cleanupRunsTotal.WithLabelValues("error").Inc()
		return
	}
	m.cleanupRunsTotal.WithLabelValues("success").Inc()
	if deleted > 0 {
		m.cleanupDeletedTotal.Add(float64(deleted))
	}
}

func (m *Metrics) RefreshLedgerIdempotencyCounts(ctx context.Context, db *sql.DB) {
	if m == nil || db == nil {
		return
	}
	const q = `
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE expires_at <= NOW()) AS expired
FROM ledger_idempotency_keys
`
	var total int64
	var expired int64
	if err := db.QueryRowContext(ctx, q).Scan(&total, &expired); err != nil {
		return
	}
	m.idempotencyKeysTotal.Set(float64(total))
	m.idempotencyKeysExpired.Set(float64(expired))
}

func (m *Metrics) ObserveIdentityLogin(result rgsv1.ResultCode, actorType rgsv1.ActorType) {
	if m == nil {
		return
	}
	outcome := "error"
	switch result {
	case rgsv1.ResultCode_RESULT_CODE_OK:
		outcome = "ok"
	case rgsv1.ResultCode_RESULT_CODE_DENIED:
		outcome = "denied"
	case rgsv1.ResultCode_RESULT_CODE_INVALID:
		outcome = "invalid"
	case rgsv1.ResultCode_RESULT_CODE_ERROR:
		outcome = "error"
	}
	m.loginAttemptsTotal.WithLabelValues(outcome, actorType.String()).Inc()
}

func (m *Metrics) ObserveIdentityLockoutActivation(actorType rgsv1.ActorType) {
	if m == nil {
		return
	}
	m.lockoutActivations.WithLabelValues(actorType.String()).Inc()
}

func (m *Metrics) RefreshIdentitySessionCounts(ctx context.Context, db *sql.DB) {
	if m == nil || db == nil {
		return
	}
	const q = `
SELECT
  COUNT(*) FILTER (WHERE revoked = FALSE AND expires_at > NOW()) AS active,
  COUNT(*) FILTER (WHERE revoked = TRUE) AS revoked,
  COUNT(*) FILTER (WHERE expires_at <= NOW()) AS expired
FROM identity_sessions
`
	var active int64
	var revoked int64
	var expired int64
	if err := db.QueryRowContext(ctx, q).Scan(&active, &revoked, &expired); err != nil {
		return
	}
	m.identitySessionsActive.Set(float64(active))
	m.identitySessionsRevoked.Set(float64(revoked))
	m.identitySessionsExpired.Set(float64(expired))
}
