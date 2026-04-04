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
	"github.com/ssovich/diasoft-gateway/internal/infrastructure/postgres"
	redisinfra "github.com/ssovich/diasoft-gateway/internal/infrastructure/redis"
	auditstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/audit"
	diplomastore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/diploma"
	sharelinkstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/sharelink"
	loggerpkg "github.com/ssovich/diasoft-gateway/internal/lib/logger"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	apptracing "github.com/ssovich/diasoft-gateway/internal/observability/tracing"
	"github.com/ssovich/diasoft-gateway/internal/transport/handler/publicverify"
	"github.com/ssovich/diasoft-gateway/internal/transport/handler/studentsharelink"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Printf("diasoft-gateway exited with error: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg := config.MustLoad()
	logger := loggerpkg.New(cfg.Log.Level)
	metricsSvc := appmetrics.New()
	shutdownTracing, err := apptracing.Setup(ctx, cfg.Tracing, cfg.Env, "diasoft-gateway-api")
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
	rateLimiter := redisinfra.NewRateLimiter(redisClient, cfg.RateLimit.Prefix, cfg.RateLimit.Requests, cfg.RateLimit.Window)
	ipResolver, err := transportmiddleware.NewClientIPResolver(cfg.HTTP.TrustedProxies)
	if err != nil {
		return fmt.Errorf("build client ip resolver: %w", err)
	}
	diplomaRepo := diplomastore.NewStore(dbPool)
	shareLinkRepo := sharelinkstore.NewStore(dbPool)
	auditRepo := auditstore.NewStore(dbPool)

	verifyUseCase := usecase.NewVerify(diplomaRepo, verifyCache)
	resolveShareLinkUseCase := usecase.NewResolveShareLink(shareLinkRepo, diplomaRepo)
	auditUseCase := usecase.NewRecordVerificationAudit(auditRepo)

	verifyHandler := publicverify.NewHTTPHandler(verifyUseCase, auditUseCase, ipResolver)
	shareLinkHandler := studentsharelink.NewHTTPHandler(resolveShareLinkUseCase, auditUseCase, ipResolver)
	publicRateLimit := transportmiddleware.NewRateLimit(rateLimiter, ipResolver, cfg.RateLimit.Window, metricsSvc)
	cors := transportmiddleware.NewCORS(cfg.HTTP.AllowedOrigins)
	securityHeaders := transportmiddleware.NewSecurityHeaders()

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/public/verify", publicRateLimit(http.HandlerFunc(verifyHandler.Verify)))
	mux.Handle("GET /api/v1/public/verify/{verificationToken}", publicRateLimit(http.HandlerFunc(verifyHandler.VerifyByToken)))
	mux.Handle("GET /api/v1/public/share-links/{shareToken}", publicRateLimit(http.HandlerFunc(shareLinkHandler.Resolve)))
	mux.Handle("GET /v/{verificationToken}", publicRateLimit(http.HandlerFunc(verifyHandler.VerifyPage)))
	mux.Handle("GET /s/{shareToken}", publicRateLimit(http.HandlerFunc(shareLinkHandler.ResolvePage)))
	mux.Handle("GET /metrics", metricsSvc.Handler())
	registerPlatformRoutes(mux, readiness(dbPool.Ping, func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }))

	server := &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           apptracing.WrapHTTP("diasoft-gateway-api", withRequestLogging(logger, cors(securityHeaders(mux)))),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("diasoft-gateway started", "address", cfg.HTTP.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	return nil
}
