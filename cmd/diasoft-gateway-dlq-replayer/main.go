package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/config"
	"github.com/ssovich/diasoft-gateway/internal/infrastructure/kafka"
	loggerpkg "github.com/ssovich/diasoft-gateway/internal/lib/logger"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	apptracing "github.com/ssovich/diasoft-gateway/internal/observability/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Printf("diasoft-gateway-dlq-replayer exited with error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg := config.MustLoad()
	logger := loggerpkg.New(cfg.Log.Level)
	metricsSvc := appmetrics.New()
	shutdownTracing, err := apptracing.Setup(ctx, cfg.Tracing, cfg.Env, "diasoft-gateway-dlq-replayer")
	if err != nil {
		return fmt.Errorf("setup tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	replayer, err := kafka.NewReplayer(ctx, logger, cfg.Kafka, metricsSvc)
	if err != nil {
		return fmt.Errorf("create kafka dlq replayer: %w", err)
	}

	metricsServer := newMetricsServer(cfg.Metrics.Address, metricsSvc.Handler(), readiness(replayer.Ping))
	go func() {
		logger.Info("diasoft-gateway-dlq-replayer metrics server started", "address", cfg.Metrics.Address)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("dlq replayer metrics server failed: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()

	return replayer.Run(ctx)
}
