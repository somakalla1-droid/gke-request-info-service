package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInfo(t *testing.T) {
	h := New(Config{AppName: "request-info", ClusterName: "cluster-a"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "test-request")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "test-request") {
		t.Fatalf("unexpected response: %d %s", res.Code, res.Body.String())
	}
}

func TestInfoCallsResponseServiceWithRequestID(t *testing.T) {
	response := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "shared-request" {
			t.Errorf("X-Request-ID = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"application": "response-service", "request_id": r.Header.Get("X-Request-ID")})
	}))
	defer response.Close()

	h := New(Config{AppName: "request-info", ResponseServiceURL: response.URL})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "shared-request")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["request_id"] != "shared-request" || body["response_status_code"] != float64(http.StatusOK) {
		t.Fatalf("unexpected response body: %#v", body)
	}
	downstream, ok := body["response_service"].(map[string]any)
	if !ok || downstream["request_id"] != "shared-request" {
		t.Fatalf("downstream response = %#v", body["response_service"])
	}
}

func TestInfoPropagatesTraceContext(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(t.Context())
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	var traceparent string
	response := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		_ = json.NewEncoder(w).Encode(map[string]string{"application": "response-service"})
	}))
	defer response.Close()

	h := New(Config{AppName: "request-info", ResponseServiceURL: response.URL})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	if traceparent == "" {
		t.Fatal("downstream request has no traceparent header")
	}
}

func TestInfoReturnsBadGatewayWhenResponseServiceFails(t *testing.T) {
	h := New(Config{ResponseServiceURL: "http://127.0.0.1:1"})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusBadGateway {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
}
func TestControlledError(t *testing.T) {
	h := New(Config{})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/error", nil))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestControlledErrorLogsStatusAndSeverity(t *testing.T) {
	var logs bytes.Buffer
	var errorLogs bytes.Buffer
	requestLogger.SetOutput(&logs)
	errorLogger.SetOutput(&errorLogs)
	t.Cleanup(func() {
		requestLogger.SetOutput(os.Stdout)
		errorLogger.SetOutput(os.Stderr)
	})

	h := New(Config{})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/error", nil))

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("request log is not JSON: %v", err)
	}
	if entry["severity"] != "ERROR" {
		t.Fatalf("severity = %v", entry["severity"])
	}
	if entry["status_code"] != float64(http.StatusInternalServerError) {
		t.Fatalf("status_code = %v", entry["status_code"])
	}
	var errorEntry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(errorLogs.Bytes()), &errorEntry); err != nil {
		t.Fatalf("error report is not JSON: %v", err)
	}
	if !strings.Contains(errorEntry["message"].(string), "goroutine") {
		t.Fatal("error report does not contain a Go stack trace")
	}
}
func TestDelayRejectsRange(t *testing.T) {
	h := New(Config{})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/delay?ms=5001", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", res.Code)
	}
}
