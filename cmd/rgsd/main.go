package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
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
	tlsEnabled := envOr("RGS_TLS_ENABLED", "false") == "true"
	tlsRequireClientCert := envOr("RGS_TLS_REQUIRE_CLIENT_CERT", "false") == "true"
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

	grpcOpts := make([]grpc.ServerOption, 0)
	if tlsCfg != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}
	grpcServer := grpc.NewServer(grpcOpts...)
	hs := health.NewServer()
	hs.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(grpcServer, hs)
	systemSvc := server.SystemService{StartedAt: startedAt, Clock: clk, Version: version}
	rgsv1.RegisterSystemServiceServer(grpcServer, systemSvc)
	ledgerSvc := server.NewLedgerService(clk)
	rgsv1.RegisterLedgerServiceServer(grpcServer, ledgerSvc)
	registrySvc := server.NewRegistryService(clk)
	rgsv1.RegisterRegistryServiceServer(grpcServer, registrySvc)
	eventsSvc := server.NewEventsService(clk)
	rgsv1.RegisterEventsServiceServer(grpcServer, eventsSvc)
	reportingSvc := server.NewReportingService(clk, ledgerSvc, eventsSvc)
	rgsv1.RegisterReportingServiceServer(grpcServer, reportingSvc)
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
	configSvc := server.NewConfigService(clk, db)
	rgsv1.RegisterConfigServiceServer(grpcServer, configSvc)

	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("listen grpc: %v", err)
	}

	mux := http.NewServeMux()
	h := server.SystemHandler{}
	h.Register(mux)
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterSystemServiceHandlerServer(ctx, gwMux, systemSvc); err != nil {
		log.Fatalf("register gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterLedgerServiceHandlerServer(ctx, gwMux, ledgerSvc); err != nil {
		log.Fatalf("register ledger gateway handlers: %v", err)
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
	remoteAccessAuditStore := audit.NewInMemoryStore()
	guard, err := server.NewRemoteAccessGuard(clk, remoteAccessAuditStore, trustedCIDRs)
	if err != nil {
		log.Fatalf("configure remote access guard: %v", err)
	}
	auditSvc := server.NewAuditService(
		clk,
		guard,
		ledgerSvc.AuditStore,
		registrySvc.AuditStore,
		eventsSvc.AuditStore,
		reportingSvc.AuditStore,
		configSvc.AuditStore,
		remoteAccessAuditStore,
	)
	rgsv1.RegisterAuditServiceServer(grpcServer, auditSvc)
	if err := rgsv1.RegisterAuditServiceHandlerServer(ctx, gwMux, auditSvc); err != nil {
		log.Fatalf("register audit gateway handlers: %v", err)
	}
	mux.Handle("/", guard.Wrap(gwMux))
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
