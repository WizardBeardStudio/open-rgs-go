package server

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	remoteAccessDecisions   *prometheus.CounterVec
	rpcRequestsTotal        *prometheus.CounterVec
	rpcRequestLatency       *prometheus.HistogramVec
	httpRequestsTotal       *prometheus.CounterVec
	httpRequestLatency      *prometheus.HistogramVec
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
		remoteAccessDecisions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "remote_access",
				Name:      "decisions_total",
				Help:      "Total remote admin boundary decisions by outcome.",
			},
			[]string{"outcome"},
		),
		rpcRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "rpc",
				Name:      "requests_total",
				Help:      "Total RPC requests partitioned by transport/method/result.",
			},
			[]string{"transport", "method", "result"},
		),
		rpcRequestLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "open_rgs",
				Subsystem: "rpc",
				Name:      "request_duration_seconds",
				Help:      "RPC request duration partitioned by transport/method.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
			},
			[]string{"transport", "method"},
		),
		httpRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "open_rgs",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total HTTP requests partitioned by method/path/status.",
			},
			[]string{"method", "path", "status"},
		),
		httpRequestLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "open_rgs",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration partitioned by method/path.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
			},
			[]string{"method", "path"},
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

func (m *Metrics) ObserveRemoteAccessDecision(outcome string) {
	if m == nil {
		return
	}
	if outcome == "" {
		outcome = "unknown"
	}
	m.remoteAccessDecisions.WithLabelValues(outcome).Inc()
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

func (m *Metrics) ObserveRPCRequest(transport string, method string, result codes.Code, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.rpcRequestsTotal.WithLabelValues(transport, method, result.String()).Inc()
	m.rpcRequestLatency.WithLabelValues(transport, method).Observe(elapsed.Seconds())
}

func (m *Metrics) ObserveHTTPRequest(method, path string, statusCode int, elapsed time.Duration) {
	if m == nil {
		return
	}
	statusClass := "5xx"
	switch {
	case statusCode >= 200 && statusCode < 300:
		statusClass = "2xx"
	case statusCode >= 300 && statusCode < 400:
		statusClass = "3xx"
	case statusCode >= 400 && statusCode < 500:
		statusClass = "4xx"
	}
	m.httpRequestsTotal.WithLabelValues(method, path, statusClass).Inc()
	m.httpRequestLatency.WithLabelValues(method, path).Observe(elapsed.Seconds())
}

func UnaryMetricsInterceptor(metrics *Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		started := time.Now()
		resp, err := handler(ctx, req)
		metrics.ObserveRPCRequest("grpc", info.FullMethod, status.Code(err), time.Since(started))
		return resp, err
	}
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func HTTPMetricsMiddleware(metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		mw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(mw, r)
		metrics.ObserveRPCRequest("rest", r.URL.Path, grpcCodeFromHTTPStatus(mw.status), time.Since(started))
		metrics.ObserveHTTPRequest(r.Method, r.URL.Path, mw.status, time.Since(started))
	})
}

func grpcCodeFromHTTPStatus(statusCode int) codes.Code {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return codes.OK
	case statusCode == http.StatusBadRequest:
		return codes.InvalidArgument
	case statusCode == http.StatusUnauthorized:
		return codes.Unauthenticated
	case statusCode == http.StatusForbidden:
		return codes.PermissionDenied
	case statusCode == http.StatusNotFound:
		return codes.NotFound
	case statusCode == http.StatusConflict:
		return codes.Aborted
	case statusCode == http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case statusCode >= 400 && statusCode < 500:
		return codes.FailedPrecondition
	default:
		return codes.Internal
	}
}
