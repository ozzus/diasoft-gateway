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

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	"github.com/ssovich/diasoft-gateway/internal/config"
	"github.com/ssovich/diasoft-gateway/internal/infrastructure/kafka"
	"github.com/ssovich/diasoft-gateway/internal/infrastructure/postgres"
	redisinfra "github.com/ssovich/diasoft-gateway/internal/infrastructure/redis"
	readmodelstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/readmodel"
	loggerpkg "github.com/ssovich/diasoft-gateway/internal/lib/logger"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	apptracing "github.com/ssovich/diasoft-gateway/internal/observability/tracing"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Printf("diasoft-gateway-consumer exited with error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg := config.MustLoad()
	logger := loggerpkg.New(cfg.Log.Level)
	metricsSvc := appmetrics.New()
	shutdownTracing, err := apptracing.Setup(ctx, cfg.Tracing, cfg.Env, "diasoft-gateway-consumer")
	if err != nil {
		return fmt.Errorf("setup tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	dbPool, err := postgres.NewPool(ctx, cfg.Database.URL, cfg.Database.MaxConns)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer dbPool.Close()

	redisClient, err := redisinfra.NewClient(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer redisClient.Close()

	verifyCache := appmetrics.NewVerificationCache(redisinfra.NewVerificationCache(redisClient, cfg.Redis.VerifyTTL), metricsSvc)
	projector := usecase.NewProjectEvents(readmodelstore.NewStore(dbPool), verifyCache)
	consumer, err := kafka.NewConsumer(ctx, logger, cfg.Kafka, projector, metricsSvc)
	if err != nil {
		return fmt.Errorf("create kafka consumer: %w", err)
	}

	metricsServer := newMetricsServer(
		cfg.Metrics.Address,
		metricsSvc.Handler(),
		readiness(
			dbPool.Ping,
			func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
			consumer.Ping,
		),
	)
	go func() {
		logger.Info("diasoft-gateway-consumer metrics server started", "address", cfg.Metrics.Address)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("consumer metrics server failed: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()

	return consumer.Run(ctx)
}
