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
	registryprivate "github.com/ssovich/diasoft-gateway/internal/infrastructure/registryprivate"
	auditstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/audit"
	authuserstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/authuser"
	diplomastore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/diploma"
	sharelinkstore "github.com/ssovich/diasoft-gateway/internal/infrastructure/storage/sharelink"
	loggerpkg "github.com/ssovich/diasoft-gateway/internal/lib/logger"
	appmetrics "github.com/ssovich/diasoft-gateway/internal/observability/metrics"
	apptracing "github.com/ssovich/diasoft-gateway/internal/observability/tracing"
	"github.com/ssovich/diasoft-gateway/internal/privateapi"
	privateauthhandler "github.com/ssovich/diasoft-gateway/internal/transport/handler/privateauth"
	privatestudenthandler "github.com/ssovich/diasoft-gateway/internal/transport/handler/privatestudent"
	privateuniversityhandler "github.com/ssovich/diasoft-gateway/internal/transport/handler/privateuniversity"
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
	authUserRepo := authuserstore.NewStore(dbPool)
	registryClient := registryprivate.NewClient(cfg.Registry.InternalBaseURL, cfg.Registry.ServiceToken, cfg.Registry.Timeout)

	verifyUseCase := usecase.NewVerify(diplomaRepo, verifyCache)
	resolveShareLinkUseCase := usecase.NewResolveShareLink(shareLinkRepo, diplomaRepo)
	auditUseCase := usecase.NewRecordVerificationAudit(auditRepo)
	privateTokenManager := privateapi.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.TokenTTL)
	privateAuthService := privateapi.NewService(authUserRepo, privateTokenManager)

	verifyHandler := publicverify.NewHTTPHandler(verifyUseCase, auditUseCase, ipResolver)
	shareLinkHandler := studentsharelink.NewHTTPHandler(resolveShareLinkUseCase, auditUseCase, ipResolver)
	authHandler := privateauthhandler.NewHTTPHandler(privateAuthService)
	universityHandler := privateuniversityhandler.NewHTTPHandler(registryClient)
	studentHandler := privatestudenthandler.NewHTTPHandler(registryClient)
	publicRateLimit := transportmiddleware.NewRateLimit(rateLimiter, ipResolver, cfg.RateLimit.Window, metricsSvc)
	privateAuth := transportmiddleware.NewPrivateAuth(privateTokenManager)
	requireUniversity := transportmiddleware.RequirePrivateRole(privateapi.RoleUniversity)
	requireStudent := transportmiddleware.RequirePrivateRole(privateapi.RoleStudent)
	cors := transportmiddleware.NewCORS(cfg.HTTP.AllowedOrigins)
	securityHeaders := transportmiddleware.NewSecurityHeaders()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.Handle("GET /api/v1/auth/me", privateAuth(http.HandlerFunc(authHandler.Me)))
	mux.Handle("GET /api/v1/university/diplomas", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.ListDiplomas))))
	mux.Handle("POST /api/v1/university/diplomas/upload", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.UploadDiplomas))))
	mux.Handle("GET /api/v1/university/imports/{jobId}", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.GetImport))))
	mux.Handle("GET /api/v1/university/imports/{jobId}/errors", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.GetImportErrors))))
	mux.Handle("POST /api/v1/university/diplomas/{id}/revoke", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.RevokeDiploma))))
	mux.Handle("GET /api/v1/university/diplomas/{id}/qr", privateAuth(requireUniversity(http.HandlerFunc(universityHandler.GetQR))))
	mux.Handle("GET /api/v1/student/diploma", privateAuth(requireStudent(http.HandlerFunc(studentHandler.GetDiploma))))
	mux.Handle("POST /api/v1/student/share-link", privateAuth(requireStudent(http.HandlerFunc(studentHandler.CreateShareLink))))
	mux.Handle("DELETE /api/v1/student/share-link/{token}", privateAuth(requireStudent(http.HandlerFunc(studentHandler.DeleteShareLink))))
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
