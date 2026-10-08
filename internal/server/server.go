package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	AppName, Version, ClusterName, Region, PodName, ResponseServiceURL string
}
type service struct {
	config           Config
	requests, errors atomic.Uint64
	client           *http.Client
}

var requestLogger = log.New(os.Stdout, "", 0)

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(statusCode int) {
	if w.statusCode == 0 {
		w.statusCode = statusCode
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.statusCode == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func New(config Config) http.Handler {
	s := &service{config: config, client: &http.Client{Timeout: 2 * time.Second}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.info)
	mux.HandleFunc("/healthz", plain("ok"))
	mux.HandleFunc("/readyz", plain("ready"))
	mux.HandleFunc("/metrics", s.metrics)
	mux.HandleFunc("/error", s.controlledError)
	mux.HandleFunc("/delay", s.delay)
	return s.logging(mux)
}

func (s *service) info(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	result := map[string]any{
		"application": s.config.AppName,
		"version":     s.config.Version,
		"cluster":     s.config.ClusterName,
		"region":      s.config.Region,
		"pod":         s.config.PodName,
		"request_id":  requestID(r),
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if s.config.ResponseServiceURL != "" {
		downstream, statusCode, latency, err := s.responseService(r)
		if err != nil {
			s.errors.Add(1)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "response service unavailable", "request_id": requestID(r)})
			return
		}
		result["response_service"] = downstream
		result["response_status_code"] = statusCode
		result["response_latency_ms"] = latency.Milliseconds()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *service) responseService(r *http.Request) (map[string]any, int, time.Duration, error) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(s.config.ResponseServiceURL, "/")+"/", nil)
	if err != nil {
		return nil, 0, 0, err
	}
	request.Header.Set("X-Request-ID", requestID(r))
	started := time.Now()
	response, err := s.client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return nil, 0, latency, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, response.StatusCode, latency, fmt.Errorf("response service returned %s", response.Status)
	}
	var body map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		return nil, response.StatusCode, latency, err
	}
	return body, response.StatusCode, latency, nil
}
func (s *service) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\nhttp_requests_total %d\n# TYPE http_errors_total counter\nhttp_errors_total %d\n", s.requests.Load(), s.errors.Load())
}
func (s *service) controlledError(w http.ResponseWriter, r *http.Request) {
	s.errors.Add(1)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "controlled observability test", "request_id": requestID(r)})
}
func (s *service) delay(w http.ResponseWriter, r *http.Request) {
	milliseconds, err := strconv.Atoi(r.URL.Query().Get("ms"))
	if err != nil || milliseconds < 0 || milliseconds > 5000 {
		http.Error(w, "ms must be between 0 and 5000", http.StatusBadRequest)
		return
	}
	time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]any{"delayed_ms": milliseconds, "request_id": requestID(r)})
}
func (s *service) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		s.requests.Add(1)
		id := requestID(r)
		w.Header().Set("X-Request-ID", id)
		recorder := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		statusCode := recorder.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		severity := "INFO"
		if statusCode >= http.StatusInternalServerError {
			severity = "ERROR"
		} else if statusCode >= http.StatusBadRequest {
			severity = "WARNING"
		}

		entry, _ := json.Marshal(map[string]any{"severity": severity, "message": "request completed", "method": r.Method, "path": r.URL.Path, "request_id": id, "status_code": statusCode, "latency_ms": time.Since(started).Milliseconds()})
		requestLogger.Print(string(entry))
	})
}
func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
func plain(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
