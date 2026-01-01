package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ServerConfig holds configuration for the metrics server.
type ServerConfig struct {
	Port        int
	MetricsPath string
	HealthPath  string
}

// DefaultServerConfig returns default server configuration.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:        9090,
		MetricsPath: "/metrics",
		HealthPath:  "/health",
	}
}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    string            `json:"uptime"`
	Checks    map[string]Check  `json:"checks"`
}

// Check represents a single health check.
type Check struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthChecker is a function that performs a health check.
type HealthChecker func() Check

// Server handles metrics and health endpoints.
type Server struct {
	cfg        ServerConfig
	httpServer *http.Server
	startTime  time.Time
	logger     *slog.Logger

	mu       sync.RWMutex
	checkers map[string]HealthChecker
}

// NewServer creates a new metrics server.
func NewServer(cfg ServerConfig, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	s := &Server{
		cfg:       cfg,
		startTime: time.Now(),
		logger:    logger,
		checkers:  make(map[string]HealthChecker),
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.MetricsPath, promhttp.Handler())
	mux.HandleFunc(cfg.HealthPath, s.healthHandler)
	mux.HandleFunc("/ready", s.readyHandler)
	mux.HandleFunc("/live", s.liveHandler)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// RegisterHealthCheck registers a health checker.
func (s *Server) RegisterHealthCheck(name string, checker HealthChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkers[name] = checker
}

// Start starts the metrics server.
func (s *Server) Start() error {
	s.logger.Info("starting metrics server",
		"port", s.cfg.Port,
		"metrics_path", s.cfg.MetricsPath,
		"health_path", s.cfg.HealthPath,
	)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("metrics server error", "err", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down metrics server")
	return s.httpServer.Shutdown(ctx)
}

// healthHandler handles the /health endpoint.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	checkers := make(map[string]HealthChecker, len(s.checkers))
	for k, v := range s.checkers {
		checkers[k] = v
	}
	s.mu.RUnlock()

	checks := make(map[string]Check)
	overallStatus := "healthy"

	for name, checker := range checkers {
		check := checker()
		checks[name] = check
		if check.Status != "healthy" {
			overallStatus = "unhealthy"
		}
	}

	status := HealthStatus{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Uptime:    time.Since(s.startTime).String(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	if overallStatus != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(status)
}

// readyHandler handles the /ready endpoint (Kubernetes readiness probe).
func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	checkers := s.checkers
	s.mu.RUnlock()

	for _, checker := range checkers {
		check := checker()
		if check.Status != "healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

// liveHandler handles the /live endpoint (Kubernetes liveness probe).
func (s *Server) liveHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("alive"))
}

// Uptime returns the server uptime.
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}
