package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"github.com/wizardbeard/open-rgs-go/internal/platform/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startedAt := time.Now().UTC()
	clk := clock.RealClock{}
	version := envOr("RGS_VERSION", "dev")
	grpcAddr := envOr("RGS_GRPC_ADDR", ":8081")
	httpAddr := envOr("RGS_HTTP_ADDR", ":8080")
	trustedCIDRs := strings.Split(envOr("RGS_TRUSTED_CIDRS", "127.0.0.1/32,::1/128"), ",")
	databaseURL := envOr("RGS_DATABASE_URL", "")
	jwtSigningSecret := envOr("RGS_JWT_SIGNING_SECRET", "dev-insecure-change-me")
	jwtKeysetSpec := envOr("RGS_JWT_KEYSET", "")
	jwtActiveKID := envOr("RGS_JWT_ACTIVE_KID", "default")
	jwtKeysetFile := envOr("RGS_JWT_KEYSET_FILE", "")
	jwtKeysetCommand := envOr("RGS_JWT_KEYSET_COMMAND", "")
	jwtKeysetRefreshInterval := mustParseDurationEnv("RGS_JWT_KEYSET_REFRESH_INTERVAL", "1m")
	downloadSigningKeysSpec := envOr("RGS_DOWNLOAD_SIGNING_KEYS", "")
	jwtAccessTTL := mustParseDurationEnv("RGS_JWT_ACCESS_TTL", "15m")
	jwtRefreshTTL := mustParseDurationEnv("RGS_JWT_REFRESH_TTL", "24h")
	identityLockoutTTL := mustParseDurationEnv("RGS_IDENTITY_LOCKOUT_TTL", "15m")
	identityLockoutMaxFailures := mustParseIntEnv("RGS_IDENTITY_LOCKOUT_MAX_FAILURES", 5)
	identitySessionCleanupInterval := mustParseDurationEnv("RGS_IDENTITY_SESSION_CLEANUP_INTERVAL", "15m")
	identitySessionCleanupBatch := mustParseIntEnv("RGS_IDENTITY_SESSION_CLEANUP_BATCH", 500)
	identityLoginRateLimitMaxAttempts := mustParseIntEnv("RGS_IDENTITY_LOGIN_RATE_LIMIT_MAX_ATTEMPTS", 60)
	identityLoginRateLimitWindow := mustParseDurationEnv("RGS_IDENTITY_LOGIN_RATE_LIMIT_WINDOW", "1m")
	eftFraudMaxFailures := mustParseIntEnv("RGS_EFT_FRAUD_MAX_FAILURES", 5)
	eftFraudLockoutTTL := mustParseDurationEnv("RGS_EFT_FRAUD_LOCKOUT_TTL", "15m")
	idempotencyTTL := mustParseDurationEnv("RGS_LEDGER_IDEMPOTENCY_TTL", "24h")
	idempotencyCleanupInterval := mustParseDurationEnv("RGS_LEDGER_IDEMPOTENCY_CLEANUP_INTERVAL", "15m")
	idempotencyCleanupBatch := mustParseIntEnv("RGS_LEDGER_IDEMPOTENCY_CLEANUP_BATCH", 500)
	metricsRefreshInterval := mustParseDurationEnv("RGS_METRICS_REFRESH_INTERVAL", "1m")
	tlsEnabled := envOr("RGS_TLS_ENABLED", "false") == "true"
	tlsRequireClientCert := envOr("RGS_TLS_REQUIRE_CLIENT_CERT", "false") == "true"
	strictProductionMode := mustParseBoolEnv("RGS_STRICT_PRODUCTION_MODE", version != "dev")
	strictExternalJWTKeyset := mustParseBoolEnv("RGS_STRICT_EXTERNAL_JWT_KEYSET", strictProductionMode)
	if err := validateProductionRuntime(strictProductionMode, strictExternalJWTKeyset, databaseURL, tlsEnabled, jwtSigningSecret, jwtKeysetSpec, jwtKeysetFile, jwtKeysetCommand); err != nil {
		log.Fatalf("invalid production runtime configuration: %v", err)
	}
	tlsCfg, err := server.BuildTLSConfig(server.TLSConfig{
		Enabled:           tlsEnabled,
		CertFile:          envOr("RGS_TLS_CERT_FILE", ""),
		KeyFile:           envOr("RGS_TLS_KEY_FILE", ""),
		ClientCAFile:      envOr("RGS_TLS_CLIENT_CA_FILE", ""),
		RequireClientCert: tlsRequireClientCert,
		MinVersionTLS12:   true,
	})
	if err != nil {
		log.Fatalf("configure tls: %v", err)
	}

	jwtKeyset, keysetFingerprint, err := loadJWTKeyset(ctx, jwtSigningSecret, jwtKeysetSpec, jwtActiveKID, jwtKeysetFile, jwtKeysetCommand)
	if err != nil {
		log.Fatalf("load jwt keyset: %v", err)
	}
	jwtSigner := platformauth.NewJWTSignerWithKeyset(jwtKeyset)
	jwtVerifier := platformauth.NewJWTVerifierWithKeyset(jwtKeyset)
	metrics := server.NewMetrics()
	grpcOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			server.UnaryMetricsInterceptor(metrics),
			platformauth.UnaryJWTInterceptor(jwtVerifier, []string{
				"/rgs.v1.SystemService/GetSystemStatus",
				"/rgs.v1.IdentityService/Login",
				"/rgs.v1.IdentityService/RefreshToken",
				"/grpc.health.v1.Health/Check",
			}),
		),
	}
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}
	var db *sql.DB
	if databaseURL != "" {
		var err error
		db, err = sql.Open("pgx", databaseURL)
		if err != nil {
			log.Fatalf("open database: %v", err)
		}
		if err := db.PingContext(ctx); err != nil {
			log.Fatalf("ping database: %v", err)
		}
		defer db.Close()
	}
	grpcServer := grpc.NewServer(grpcOpts...)
	hs := health.NewServer()
	hs.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(grpcServer, hs)
	systemSvc := server.SystemService{StartedAt: startedAt, Clock: clk, Version: version}
	rgsv1.RegisterSystemServiceServer(grpcServer, systemSvc)
	identitySvc := server.NewIdentityService(clk, jwtSigningSecret, jwtAccessTTL, jwtRefreshTTL, db)
	identitySvc.SetJWTSigner(jwtSigner)
	identitySvc.SetLockoutPolicy(identityLockoutMaxFailures, identityLockoutTTL)
	identitySvc.SetLoginRateLimit(identityLoginRateLimitMaxAttempts, identityLoginRateLimitWindow)
	identitySvc.StartSessionCleanupWorker(ctx, identitySessionCleanupInterval, identitySessionCleanupBatch, log.Printf)
	if (strings.TrimSpace(jwtKeysetFile) != "" || strings.TrimSpace(jwtKeysetCommand) != "") && jwtKeysetRefreshInterval > 0 {
		go func() {
			ticker := time.NewTicker(jwtKeysetRefreshInterval)
			defer ticker.Stop()
			currentFingerprint := keysetFingerprint
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					loaded, fingerprint, err := loadJWTKeyset(ctx, jwtSigningSecret, jwtKeysetSpec, jwtActiveKID, jwtKeysetFile, jwtKeysetCommand)
					if err != nil {
						log.Printf("jwt keyset refresh failed: %v", err)
						continue
					}
					if fingerprint == currentFingerprint {
						continue
					}
					if err := jwtSigner.SetKeyset(loaded); err != nil {
						log.Printf("jwt signer keyset refresh failed: %v", err)
						continue
					}
					if err := jwtVerifier.SetKeyset(loaded); err != nil {
						log.Printf("jwt verifier keyset refresh failed: %v", err)
						continue
					}
					currentFingerprint = fingerprint
					log.Printf("jwt keyset reloaded (active_kid=%s)", loaded.ActiveKID)
				}
			}
		}()
	}
	if db != nil {
		ok, err := identitySvc.HasActiveCredentials(ctx)
		if err != nil {
			log.Fatalf("verify bootstrap identity credentials: %v", err)
		}
		if !ok {
			log.Fatalf("no active identity credentials found; seed identity_credentials before startup")
		}
	}
	rgsv1.RegisterIdentityServiceServer(grpcServer, identitySvc)
	ledgerSvc := server.NewLedgerService(clk, db)
	ledgerSvc.SetEFTFraudPolicy(eftFraudMaxFailures, eftFraudLockoutTTL)
	ledgerSvc.SetDisableInMemoryIdempotencyCache(strictProductionMode)
	identitySvc.SetMetricsObservers(metrics.ObserveIdentityLogin, metrics.ObserveIdentityLockoutActivation)
	if db != nil {
		metrics.RefreshLedgerIdempotencyCounts(ctx, db)
		metrics.RefreshIdentitySessionCounts(ctx, db)
		if metricsRefreshInterval > 0 {
			go func() {
				ticker := time.NewTicker(metricsRefreshInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						metrics.RefreshLedgerIdempotencyCounts(ctx, db)
						metrics.RefreshIdentitySessionCounts(ctx, db)
					}
				}
			}()
		}
	}
	ledgerSvc.SetIdempotencyTTL(idempotencyTTL)
	ledgerSvc.StartIdempotencyCleanupWorker(ctx, idempotencyCleanupInterval, idempotencyCleanupBatch, log.Printf, func(deleted int64, err error) {
		metrics.ObserveLedgerIdempotencyCleanup(deleted, err)
		if db != nil {
			metrics.RefreshLedgerIdempotencyCounts(ctx, db)
			metrics.RefreshIdentitySessionCounts(ctx, db)
		}
	})
	rgsv1.RegisterLedgerServiceServer(grpcServer, ledgerSvc)
	wageringSvc := server.NewWageringService(clk, db)
	wageringSvc.SetDisableInMemoryIdempotencyCache(strictProductionMode)
	rgsv1.RegisterWageringServiceServer(grpcServer, wageringSvc)
	registrySvc := server.NewRegistryService(clk, db)
	rgsv1.RegisterRegistryServiceServer(grpcServer, registrySvc)
	eventsSvc := server.NewEventsService(clk, db)
	rgsv1.RegisterEventsServiceServer(grpcServer, eventsSvc)
	reportingSvc := server.NewReportingService(clk, ledgerSvc, eventsSvc, db)
	reportingSvc.SetDisableInMemoryCache(strictProductionMode)
	rgsv1.RegisterReportingServiceServer(grpcServer, reportingSvc)
	configSvc := server.NewConfigService(clk, db)
	configSvc.SetDisableInMemoryCache(strictProductionMode)
	configSvc.SetDownloadSignatureKeys(parseKeyValueSecrets(downloadSigningKeysSpec))
	rgsv1.RegisterConfigServiceServer(grpcServer, configSvc)
	promotionsSvc := server.NewPromotionsService(clk, db)
	promotionsSvc.SetDisableInMemoryCache(strictProductionMode)
	rgsv1.RegisterPromotionsServiceServer(grpcServer, promotionsSvc)
	uiOverlaySvc := server.NewUISystemOverlayService(clk, db)
	uiOverlaySvc.SetDisableInMemoryCache(strictProductionMode)
	rgsv1.RegisterUISystemOverlayServiceServer(grpcServer, uiOverlaySvc)

	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen grpc: %v", err)
	}

	mux := http.NewServeMux()
	h := server.SystemHandler{}
	h.Register(mux)
	mux.Handle("/metrics", promhttp.Handler())
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterSystemServiceHandlerServer(ctx, gwMux, systemSvc); err != nil {
		log.Fatalf("register gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterIdentityServiceHandlerServer(ctx, gwMux, identitySvc); err != nil {
		log.Fatalf("register identity gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterLedgerServiceHandlerServer(ctx, gwMux, ledgerSvc); err != nil {
		log.Fatalf("register ledger gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterWageringServiceHandlerServer(ctx, gwMux, wageringSvc); err != nil {
		log.Fatalf("register wagering gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterRegistryServiceHandlerServer(ctx, gwMux, registrySvc); err != nil {
		log.Fatalf("register registry gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterEventsServiceHandlerServer(ctx, gwMux, eventsSvc); err != nil {
		log.Fatalf("register events gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterReportingServiceHandlerServer(ctx, gwMux, reportingSvc); err != nil {
		log.Fatalf("register reporting gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterConfigServiceHandlerServer(ctx, gwMux, configSvc); err != nil {
		log.Fatalf("register config gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterPromotionsServiceHandlerServer(ctx, gwMux, promotionsSvc); err != nil {
		log.Fatalf("register promotions gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterUISystemOverlayServiceHandlerServer(ctx, gwMux, uiOverlaySvc); err != nil {
		log.Fatalf("register ui overlay gateway handlers: %v", err)
	}
	remoteAccessAuditStore := audit.NewInMemoryStore()
	guard, err := server.NewRemoteAccessGuard(clk, remoteAccessAuditStore, trustedCIDRs)
	if err != nil {
		log.Fatalf("configure remote access guard: %v", err)
	}
	if db != nil {
		guard.SetDB(db)
	}
	guard.SetDisableInMemoryActivityCache(strictProductionMode)
	auditSvc := server.NewAuditService(
		clk,
		guard,
		ledgerSvc.AuditStore,
		registrySvc.AuditStore,
		eventsSvc.AuditStore,
		reportingSvc.AuditStore,
		configSvc.AuditStore,
		identitySvc.AuditStore,
		promotionsSvc.AuditStore,
		uiOverlaySvc.AuditStore,
		remoteAccessAuditStore,
	)
	rgsv1.RegisterAuditServiceServer(grpcServer, auditSvc)
	if err := rgsv1.RegisterAuditServiceHandlerServer(ctx, gwMux, auditSvc); err != nil {
		log.Fatalf("register audit gateway handlers: %v", err)
	}
	authenticatedGateway := platformauth.HTTPJWTMiddlewareWithSkips(jwtVerifier, gwMux, []string{
		"/v1/system/status",
		"/v1/identity/login",
		"/v1/identity/refresh",
	})
	mux.Handle("/", guard.Wrap(server.HTTPMetricsMiddleware(metrics, authenticatedGateway)))
	httpServer := &http.Server{Addr: httpAddr, Handler: mux, TLSConfig: tlsCfg}

	go func() {
		log.Printf("grpc listening on %s", grpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Printf("grpc server stopped: %v", err)
		}
	}()

	go func() {
		log.Printf("http listening on %s", httpAddr)
		var err error
		if tlsCfg != nil {
			err = httpServer.ListenAndServeTLS("", "")
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Printf("http server stopped: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

func envOr(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func mustParseDurationEnv(key, def string) time.Duration {
	raw := envOr(key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("invalid duration for %s=%q: %v", key, raw, err)
	}
	return d
}

func mustParseIntEnv(key string, def int) int {
	raw := envOr(key, "")
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Fatalf("invalid integer for %s=%q: %v", key, raw, err)
	}
	return v
}

func mustParseBoolEnv(key string, def bool) bool {
	raw := strings.TrimSpace(envOr(key, ""))
	if raw == "" {
		return def
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Fatalf("invalid boolean for %s=%q", key, raw)
		return def
	}
}

func validateProductionRuntime(strict bool, strictExternalJWTKeyset bool, databaseURL string, tlsEnabled bool, jwtSigningSecret string, jwtKeysetSpec string, jwtKeysetFile string, jwtKeysetCommand string) error {
	if !strict {
		return nil
	}
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("RGS_DATABASE_URL is required when RGS_STRICT_PRODUCTION_MODE=true")
	}
	if !tlsEnabled {
		return fmt.Errorf("RGS_TLS_ENABLED must be true when RGS_STRICT_PRODUCTION_MODE=true")
	}
	if strings.TrimSpace(jwtKeysetSpec) == "" && strings.TrimSpace(jwtKeysetFile) == "" && strings.TrimSpace(jwtKeysetCommand) == "" && jwtSigningSecret == "dev-insecure-change-me" {
		return fmt.Errorf("default JWT signing secret is not allowed when RGS_STRICT_PRODUCTION_MODE=true")
	}
	if strictExternalJWTKeyset && strings.TrimSpace(jwtKeysetFile) == "" && strings.TrimSpace(jwtKeysetCommand) == "" {
		return fmt.Errorf("RGS_JWT_KEYSET_FILE or RGS_JWT_KEYSET_COMMAND is required when RGS_STRICT_EXTERNAL_JWT_KEYSET=true")
	}
	return nil
}

func loadJWTKeyset(ctx context.Context, jwtSigningSecret string, jwtKeysetSpec string, jwtActiveKID string, jwtKeysetFile string, jwtKeysetCommand string) (platformauth.HMACKeyset, string, error) {
	if strings.TrimSpace(jwtKeysetFile) != "" {
		keyset, err := platformauth.LoadHMACKeysetFile(jwtKeysetFile)
		if err != nil {
			return platformauth.HMACKeyset{}, "", err
		}
		return keyset, keysetFingerprint(keyset), nil
	}
	if strings.TrimSpace(jwtKeysetCommand) != "" {
		keyset, err := platformauth.LoadHMACKeysetCommand(ctx, jwtKeysetCommand)
		if err != nil {
			return platformauth.HMACKeyset{}, "", err
		}
		return keyset, keysetFingerprint(keyset), nil
	}
	keyset, err := platformauth.ParseHMACKeyset(jwtSigningSecret, jwtKeysetSpec, jwtActiveKID)
	if err != nil {
		return platformauth.HMACKeyset{}, "", err
	}
	return keyset, keysetFingerprint(keyset), nil
}

func parseKeyValueSecrets(spec string) map[string][]byte {
	out := make(map[string][]byte)
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		pair := strings.SplitN(entry, ":", 2)
		if len(pair) != 2 {
			continue
		}
		kid := strings.TrimSpace(pair[0])
		secret := strings.TrimSpace(pair[1])
		if kid == "" || secret == "" {
			continue
		}
		out[kid] = []byte(secret)
	}
	return out
}

func keysetFingerprint(keyset platformauth.HMACKeyset) string {
	keys := make([]string, 0, len(keyset.Keys))
	for kid := range keyset.Keys {
		keys = append(keys, kid)
	}
	sort.Strings(keys)
	joined := keyset.ActiveKID
	for _, kid := range keys {
		joined += "|" + kid + ":" + string(keyset.Keys[kid])
	}
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}
