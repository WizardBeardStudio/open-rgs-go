package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	cleanupRunsTotal       *prometheus.CounterVec
	cleanupDeletedTotal    prometheus.Counter
	cleanupLastDeleted     prometheus.Gauge
	cleanupLastRunUnix     prometheus.Gauge
	idempotencyKeysTotal   prometheus.Gauge
	idempotencyKeysExpired prometheus.Gauge
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
