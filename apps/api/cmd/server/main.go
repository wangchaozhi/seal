package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"

	"sealplatform/api/internal/config"
	"sealplatform/api/internal/httpx"
	"sealplatform/api/internal/platform"
	"sealplatform/api/internal/raster"
	"sealplatform/api/internal/seal"
	"sealplatform/api/internal/storage"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if cfg.Environment == "production" && (len(cfg.PaymentSecret) < 32 || cfg.PaymentSecret == "development-payment-secret-change-me") {
		logger.Error("APP_PAYMENT_CALLBACK_SECRET must be a non-default secret of at least 32 bytes in production")
		os.Exit(1)
	}

	mux := http.NewServeMux()
	metrics := httpx.NewMetrics()
	mux.HandleFunc("GET /api/v1/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"status":      "ok",
			"service":     "seal-platform-api",
			"environment": cfg.Environment,
			"time":        time.Now().UTC(),
		})
	})
	seal.Handler{Renderer: seal.Renderer{}, MaxBodyBytes: cfg.MaxBodyBytes}.Register(mux)
	var stateStore *platform.StateStore
	var err error
	if cfg.DatabaseURL != "" {
		stateStore, err = platform.NewPostgresStateStore(context.Background(), cfg.DatabaseURL)
	} else {
		stateStore, err = platform.NewStateStore(filepath.Join(cfg.DataDir, "state.json"))
	}
	if err != nil {
		logger.Error("state store initialization failed", "error", err)
		os.Exit(1)
	}
	defer stateStore.Close()
	objectStore, err := storage.NewLocal(filepath.Join(cfg.DataDir, "objects"))
	if err != nil {
		logger.Error("object store initialization failed", "error", err)
		os.Exit(1)
	}
	var redisClient *redis.Client
	var generationQueue platform.GenerationQueue
	if cfg.RedisURL != "" {
		options, parseErr := redis.ParseURL(cfg.RedisURL)
		if parseErr != nil {
			logger.Error("redis URL invalid", "error", parseErr)
			os.Exit(1)
		}
		redisClient = redis.NewClient(options)
		if pingErr := httpx.PingRedis(context.Background(), redisClient); pingErr != nil {
			logger.Error("redis initialization failed", "error", pingErr)
			os.Exit(1)
		}
		defer redisClient.Close()
		generationQueue = platform.NewRedisGenerationQueue(redisClient, "seal:generation:queue")
	}
	var platformService *platform.Service
	if generationQueue != nil {
		platformService = platform.NewServiceWithQueue(stateStore, objectStore, logger, cfg.MaxBodyBytes, generationQueue)
	} else {
		platformService = platform.NewService(stateStore, objectStore, logger, cfg.MaxBodyBytes)
	}
	platformService.TokenTTL = time.Duration(cfg.DownloadTokenTTL) * time.Second
	platformService.PaymentSecret = cfg.PaymentSecret
	platformService.AdminEmail = cfg.AdminEmail
	platformService.AdminMFASecret = cfg.AdminMFASecret
	platformService.Origin = cfg.Origin
	platformService.QQOAuth = platform.DefaultQQOAuthConfig(cfg.QQAppID, cfg.QQAppSecret, cfg.QQRedirectURL, cfg.Origin)
	platformService.WeChatOAuth = platform.DefaultWeChatOAuthConfig(cfg.WeChatAppID, cfg.WeChatAppSecret, cfg.WeChatRedirectURL, cfg.Origin)
	platformService.GitHubOAuth = platform.DefaultGitHubOAuthConfig(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL, cfg.Origin)
	platformService.GoogleOAuth = platform.DefaultGoogleOAuthConfig(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL, cfg.Origin)
	platformService.RequireAdminMFA = cfg.Environment == "production"
	if platformService.RequireAdminMFA && cfg.AdminEmail != "" && cfg.AdminMFASecret == "" {
		logger.Error("APP_ADMIN_MFA_SECRET is required when an admin email is configured in production")
		os.Exit(1)
	}
	platformService.AllowPaymentSimulation = cfg.PaymentSimulation && cfg.Environment != "production"
	if cfg.RasterizerURL != "" {
		workerClient := raster.NewHTTPClient(cfg.RasterizerURL)
		platformService.Rasterizer = workerClient
		platformService.ImageProcessor = workerClient
	}
	platformService.Register(mux)
	mux.HandleFunc("GET /metrics", func(writer http.ResponseWriter, request *http.Request) {
		metrics.Handler(writer, request)
		platformService.WriteMetrics(writer)
	})

	limiter := httpx.NewRateLimiter(cfg.RateLimitPerMinute, time.Minute)
	routeLimits := map[string]int{"POST /api/v1/generations": 3, "POST /api/v1/uploads/images": 10, "POST /api/v1/orders": 5, "POST /api/v1/generations/{id}/download-token": 10, "POST /api/v1/payments/callback": 60, "POST /api/v1/auth/login": 10, "POST /api/v1/auth/register": 5}
	var routeLimitMiddleware httpx.Middleware
	if redisClient != nil {
		routeLimitMiddleware = httpx.NewRedisRouteRateLimiter(redisClient, routeLimits, time.Minute).Middleware
	} else {
		routeLimitMiddleware = httpx.NewRouteRateLimiter(routeLimits, time.Minute).Middleware
	}
	handler := httpx.Chain(
		mux,
		httpx.RequestID,
		httpx.SecurityHeaders(cfg.Origin),
		httpx.OriginGuard(cfg.Origin),
		httpx.NoStoreAPI,
		httpx.Recover(logger),
		httpx.AccessLog(logger),
		metrics.Middleware,
		limiter.Middleware,
		routeLimitMiddleware,
	)

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server starting", "addr", cfg.Addr, "origin", cfg.Origin)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
