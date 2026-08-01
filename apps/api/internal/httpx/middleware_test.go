package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientIPOnlyTrustsPrivateProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	request.RemoteAddr = "203.0.113.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(request); got != "203.0.113.10" {
		t.Fatalf("trusted public sender header: %s", got)
	}
	request.RemoteAddr = "10.0.0.4:1234"
	if got := clientIP(request); got != "198.51.100.7" {
		t.Fatalf("did not trust internal proxy: %s", got)
	}
}

func TestRouteRateLimiter(t *testing.T) {
	limiter := NewRouteRateLimiter(map[string]int{"POST /limited": 2}, time.Minute)
	handler := limiter.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	for index := 0; index < 3; index++ {
		request := httptest.NewRequest(http.MethodPost, "http://example.test/limited", nil)
		request.RemoteAddr = "203.0.113.20:1234"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		expected := http.StatusNoContent
		if index == 2 {
			expected = http.StatusTooManyRequests
		}
		if recorder.Code != expected {
			t.Fatalf("request %d status %d", index, recorder.Code)
		}
	}
}

func TestOriginGuardRejectsCrossSiteMutation(t *testing.T) {
	handler := OriginGuard("https://seal.example")(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodPost, "https://api.example/api/v1/orders", nil)
	request.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-site mutation status %d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "https://api.example/api/v1/orders", nil)
	request.Header.Set("Origin", "https://seal.example")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("same-origin mutation status %d", recorder.Code)
	}
}

func TestMetricsAndNoStore(t *testing.T) {
	metrics := NewMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/test", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusCreated) })
	handler := NoStoreAPI(metrics.Middleware(mux))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/api/v1/test", nil))
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API response is cacheable")
	}
	metricsRecorder := httptest.NewRecorder()
	metrics.Handler(metricsRecorder, httptest.NewRequest(http.MethodGet, "http://example.test/metrics", nil))
	if body := metricsRecorder.Body.String(); !strings.Contains(body, `status="201"`) {
		t.Fatalf("metrics missing status: %s", body)
	}
}
