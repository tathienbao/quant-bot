package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.MetricsPath != "/metrics" {
		t.Errorf("MetricsPath = %s, want /metrics", cfg.MetricsPath)
	}
	if cfg.HealthPath != "/health" {
		t.Errorf("HealthPath = %s, want /health", cfg.HealthPath)
	}
}

func TestServer_HealthHandler(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	// Register a healthy check
	server.RegisterHealthCheck("test", func() Check {
		return Check{Status: "healthy", Message: "all good"}
	})

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var status HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if status.Status != "healthy" {
		t.Errorf("status = %s, want healthy", status.Status)
	}
	if len(status.Checks) != 1 {
		t.Errorf("checks count = %d, want 1", len(status.Checks))
	}
	if status.Checks["test"].Status != "healthy" {
		t.Errorf("test check status = %s, want healthy", status.Checks["test"].Status)
	}
}

func TestServer_HealthHandler_Unhealthy(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	// Register an unhealthy check
	server.RegisterHealthCheck("failing", func() Check {
		return Check{Status: "unhealthy", Message: "connection lost"}
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.healthHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var status HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if status.Status != "unhealthy" {
		t.Errorf("status = %s, want unhealthy", status.Status)
	}
}

func TestServer_ReadyHandler(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	// Register healthy check
	server.RegisterHealthCheck("ready", func() Check {
		return Check{Status: "healthy"}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.readyHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "ready" {
		t.Errorf("body = %s, want ready", w.Body.String())
	}
}

func TestServer_ReadyHandler_NotReady(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	// Register unhealthy check
	server.RegisterHealthCheck("not_ready", func() Check {
		return Check{Status: "unhealthy"}
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.readyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestServer_LiveHandler(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	server.liveHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "alive" {
		t.Errorf("body = %s, want alive", w.Body.String())
	}
}

func TestServer_Uptime(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	time.Sleep(10 * time.Millisecond)

	uptime := server.Uptime()
	if uptime < 10*time.Millisecond {
		t.Errorf("uptime = %v, expected >= 10ms", uptime)
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	cfg := ServerConfig{
		Port:        19090, // Use non-standard port for testing
		MetricsPath: "/metrics",
		HealthPath:  "/health",
	}
	server := NewServer(cfg, nil)

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}
}

func TestServer_RegisterMultipleChecks(t *testing.T) {
	cfg := DefaultServerConfig()
	server := NewServer(cfg, nil)

	server.RegisterHealthCheck("check1", func() Check {
		return Check{Status: "healthy"}
	})
	server.RegisterHealthCheck("check2", func() Check {
		return Check{Status: "healthy"}
	})
	server.RegisterHealthCheck("check3", func() Check {
		return Check{Status: "degraded", Message: "slow"}
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.healthHandler(w, req)

	var status HealthStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(status.Checks) != 3 {
		t.Errorf("checks count = %d, want 3", len(status.Checks))
	}
}
