package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
	requestLogger.SetOutput(&logs)
	t.Cleanup(func() { requestLogger.SetOutput(os.Stdout) })

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
}
func TestDelayRejectsRange(t *testing.T) {
	h := New(Config{})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/delay?ms=5001", nil))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", res.Code)
	}
}
