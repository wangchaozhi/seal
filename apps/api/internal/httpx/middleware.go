package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Middleware func(http.Handler) http.Handler

func Chain(handler http.Handler, middleware ...Middleware) http.Handler {
	for index := len(middleware) - 1; index >= 0; index-- {
		handler = middleware[index](handler)
	}
	return handler
}

func OriginGuard(origin string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			unsafe := request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions
			provided := request.Header.Get("Origin")
			if unsafe && ((provided != "" && provided != origin) || request.Header.Get("Sec-Fetch-Site") == "cross-site") {
				http.Error(writer, "cross-site request rejected", http.StatusForbidden)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func NoStoreAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writer.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(writer, request)
	})
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if requestID == "" {
			bytes := make([]byte, 12)
			if _, err := rand.Read(bytes); err == nil {
				requestID = hex.EncodeToString(bytes)
			} else {
				requestID = time.Now().UTC().Format("20060102150405.000000000")
			}
		}
		writer.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(writer, request)
	})
}

func SecurityHeaders(origin string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("X-Content-Type-Options", "nosniff")
			writer.Header().Set("X-Frame-Options", "DENY")
			writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			if origin != "" && request.Header.Get("Origin") == origin {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
				writer.Header().Set("Vary", "Origin")
			}
			if request.Method == http.MethodOptions {
				writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID, X-Client-Version")
				writer.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
}

func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", "error", recovered, "path", request.URL.Path)
					http.Error(writer, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(writer, request)
		})
	}
}

func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			started := time.Now()
			next.ServeHTTP(writer, request)
			logger.Info("request", "method", request.Method, "path", request.URL.Path, "duration", time.Since(started))
		})
	}
}

type limiterEntry struct {
	windowStart time.Time
	count       int
}

type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	entries  map[string]limiterEntry
	interval time.Duration
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, interval: interval, entries: make(map[string]limiterEntry)}
}

func (limiter *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := clientIP(request)
		now := time.Now()

		limiter.mu.Lock()
		entry := limiter.entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= limiter.interval {
			entry = limiterEntry{windowStart: now, count: 0}
		}
		entry.count++
		limiter.entries[key] = entry
		if len(limiter.entries) > 10000 {
			for candidate, value := range limiter.entries {
				if now.Sub(value.windowStart) >= limiter.interval {
					delete(limiter.entries, candidate)
				}
			}
		}
		allowed := entry.count <= limiter.limit
		limiter.mu.Unlock()

		if !allowed {
			writer.Header().Set("Retry-After", "60")
			http.Error(writer, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func clientIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	remote := net.ParseIP(host)
	if remote != nil && (remote.IsLoopback() || remote.IsPrivate()) {
		if forwarded := request.Header.Get("X-Forwarded-For"); forwarded != "" {
			candidate := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	return host
}

type RouteRateLimiter struct {
	mu       sync.Mutex
	limits   map[string]int
	entries  map[string]limiterEntry
	interval time.Duration
}

type RedisRouteRateLimiter struct {
	client   *redis.Client
	limits   map[string]int
	interval time.Duration
}

func NewRedisRouteRateLimiter(client *redis.Client, limits map[string]int, interval time.Duration) *RedisRouteRateLimiter {
	return &RedisRouteRateLimiter{client: client, limits: limits, interval: interval}
}

var redisRateScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return count
`)

func (limiter *RedisRouteRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		route := rateLimitRoute(request)
		limit := limiter.limits[route]
		if limit == 0 {
			next.ServeHTTP(writer, request)
			return
		}
		key := "seal:rl:" + tokenSafe(route+":"+clientIP(request))
		count, err := redisRateScript.Run(request.Context(), limiter.client, []string{key}, limiter.interval.Milliseconds()).Int64()
		if err != nil {
			http.Error(writer, "rate limiter unavailable", http.StatusServiceUnavailable)
			return
		}
		if count > int64(limit) {
			writer.Header().Set("Retry-After", strconv.Itoa(int(limiter.interval.Seconds())))
			http.Error(writer, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func tokenSafe(value string) string {
	value = strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(value)
	if len(value) > 180 {
		return value[:180]
	}
	return value
}

func rateLimitRoute(request *http.Request) string {
	path := request.URL.Path
	if request.Method == http.MethodPost && strings.HasPrefix(path, "/api/v1/generations/") && strings.HasSuffix(path, "/download-token") {
		path = "/api/v1/generations/{id}/download-token"
	}
	return request.Method + " " + path
}

type Metrics struct {
	mu       sync.Mutex
	requests map[string]uint64
	duration map[string]time.Duration
}

func NewMetrics() *Metrics {
	return &Metrics{requests: map[string]uint64{}, duration: map[string]time.Duration{}}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (writer *statusWriter) WriteHeader(status int) {
	if writer.wrote {
		return
	}
	writer.wrote = true
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusWriter) Write(content []byte) (int, error) {
	if !writer.wrote {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(content)
}

func (metrics *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(wrapped, request)
		pattern := request.Pattern
		if pattern == "" {
			pattern = request.Method + " unmatched"
		}
		key := pattern + `",status="` + strconv.Itoa(wrapped.status)
		metrics.mu.Lock()
		metrics.requests[key]++
		metrics.duration[pattern] += time.Since(started)
		metrics.mu.Unlock()
	})
}

func (metrics *Metrics) Handler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	_, _ = fmt.Fprintln(writer, "# HELP seal_http_requests_total HTTP requests by route and status.")
	_, _ = fmt.Fprintln(writer, "# TYPE seal_http_requests_total counter")
	for key, count := range metrics.requests {
		_, _ = fmt.Fprintf(writer, "seal_http_requests_total{route=\"%s\"} %d\n", key, count)
	}
	_, _ = fmt.Fprintln(writer, "# HELP seal_http_request_duration_seconds_total Total HTTP request duration by route.")
	_, _ = fmt.Fprintln(writer, "# TYPE seal_http_request_duration_seconds_total counter")
	for route, duration := range metrics.duration {
		_, _ = fmt.Fprintf(writer, "seal_http_request_duration_seconds_total{route=\"%s\"} %.6f\n", route, duration.Seconds())
	}
}

func PingRedis(ctx context.Context, client *redis.Client) error { return client.Ping(ctx).Err() }

func NewRouteRateLimiter(limits map[string]int, interval time.Duration) *RouteRateLimiter {
	return &RouteRateLimiter{limits: limits, entries: map[string]limiterEntry{}, interval: interval}
}
func (limiter *RouteRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		route := rateLimitRoute(request)
		limit := limiter.limits[route]
		if limit == 0 {
			next.ServeHTTP(writer, request)
			return
		}
		key := route + ":" + clientIP(request)
		now := time.Now()
		limiter.mu.Lock()
		entry := limiter.entries[key]
		if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= limiter.interval {
			entry = limiterEntry{windowStart: now}
		}
		entry.count++
		limiter.entries[key] = entry
		allowed := entry.count <= limit
		limiter.mu.Unlock()
		if !allowed {
			writer.Header().Set("Retry-After", fmt.Sprintf("%d", int(limiter.interval.Seconds())))
			http.Error(writer, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(writer, request)
	})
}
