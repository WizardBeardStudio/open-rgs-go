package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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

	grpcServer := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthv1.RegisterHealthServer(grpcServer, hs)
	systemSvc := server.SystemService{StartedAt: startedAt, Clock: clk, Version: version}
	rgsv1.RegisterSystemServiceServer(grpcServer, systemSvc)

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
	mux.Handle("/", gwMux)
	httpServer := &http.Server{Addr: httpAddr, Handler: mux}

	go func() {
		log.Printf("grpc listening on %s", grpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Printf("grpc server stopped: %v", err)
		}
	}()

	go func() {
		log.Printf("http listening on %s", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
