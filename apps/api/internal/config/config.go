package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr               string
	Origin             string
	Environment        string
	MaxBodyBytes       int64
	RateLimitPerMinute int
	DataDir            string
	DownloadTokenTTL   int
	PaymentSecret      string
	PaymentSimulation  bool
	RasterizerURL      string
	AdminEmail         string
	DatabaseURL        string
	RedisURL           string
	AdminMFASecret     string
}

func Load() Config {
	return Config{
		Addr:               value("APP_ADDR", ":8080"),
		Origin:             value("APP_ORIGIN", "http://localhost:5173"),
		Environment:        value("APP_ENV", "development"),
		MaxBodyBytes:       int64Value("APP_MAX_BODY_BYTES", 1<<20),
		RateLimitPerMinute: intValue("APP_RATE_LIMIT_PER_MINUTE", 120),
		DataDir:            value("APP_DATA_DIR", "./tmp/data"),
		DownloadTokenTTL:   intValue("APP_DOWNLOAD_TOKEN_TTL_SECONDS", 120),
		PaymentSecret:      value("APP_PAYMENT_CALLBACK_SECRET", "development-payment-secret-change-me"),
		PaymentSimulation:  boolValue("APP_ENABLE_PAYMENT_SIMULATION", true),
		RasterizerURL:      value("APP_RASTERIZER_URL", ""),
		AdminEmail:         value("APP_ADMIN_EMAIL", "admin@example.com"),
		DatabaseURL:        os.Getenv("APP_DATABASE_URL"),
		RedisURL:           os.Getenv("APP_REDIS_URL"),
		AdminMFASecret:     os.Getenv("APP_ADMIN_MFA_SECRET"),
	}
}

func boolValue(key string, fallback bool) bool {
	result := os.Getenv(key)
	if result == "" {
		return fallback
	}
	value, err := strconv.ParseBool(result)
	if err != nil {
		return fallback
	}
	return value
}

func value(key, fallback string) string {
	if result := os.Getenv(key); result != "" {
		return result
	}
	return fallback
}

func intValue(key string, fallback int) int {
	result, err := strconv.Atoi(os.Getenv(key))
	if err != nil || result <= 0 {
		return fallback
	}
	return result
}

func int64Value(key string, fallback int64) int64 {
	result, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || result <= 0 {
		return fallback
	}
	return result
}
