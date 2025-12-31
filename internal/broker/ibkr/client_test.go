package ibkr

import (
	"context"
	"testing"
	"time"

	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/types"
)

// TestNewClient tests client constructor.
func TestNewClient(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	if client == nil {
		t.Fatal("expected client to be created")
	}

	if client.State() != broker.StateDisconnected {
		t.Errorf("expected state Disconnected, got %v", client.State())
	}

	if client.IsConnected() {
		t.Error("expected client to not be connected initially")
	}
}

// TestClient_DefaultConfig tests default configuration.
func TestClient_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Host)
	}

	if cfg.Port != 7497 {
		t.Errorf("expected port 7497, got %d", cfg.Port)
	}

	if cfg.ClientID != 1 {
		t.Errorf("expected clientID 1, got %d", cfg.ClientID)
	}

	if cfg.MaxRequestsPerSecond != 45 {
		t.Errorf("expected rate limit 45, got %d", cfg.MaxRequestsPerSecond)
	}

	if !cfg.AutoReconnect {
		t.Error("expected AutoReconnect to be true")
	}

	if !cfg.PaperTrading {
		t.Error("expected PaperTrading to be true by default")
	}
}

// TestClient_LiveConfig tests live configuration.
func TestClient_LiveConfig(t *testing.T) {
	cfg := LiveConfig()

	if cfg.Port != 7496 {
		t.Errorf("expected live port 7496, got %d", cfg.Port)
	}

	if cfg.PaperTrading {
		t.Error("expected PaperTrading to be false for live config")
	}
}

// TestClient_GatewayConfig tests gateway configuration.
func TestClient_GatewayConfig(t *testing.T) {
	// Paper gateway
	paperCfg := GatewayConfig(true)
	if paperCfg.Port != 4002 {
		t.Errorf("expected paper gateway port 4002, got %d", paperCfg.Port)
	}

	// Live gateway
	liveCfg := GatewayConfig(false)
	if liveCfg.Port != 4001 {
		t.Errorf("expected live gateway port 4001, got %d", liveCfg.Port)
	}
}

// TestClient_State tests state transitions.
func TestClient_State(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	// Initially disconnected
	if client.State() != broker.StateDisconnected {
		t.Errorf("expected initial state Disconnected, got %v", client.State())
	}

	// IsConnected should be false
	if client.IsConnected() {
		t.Error("expected IsConnected to return false")
	}
}

// TestClient_Connect_Timeout tests connection timeout handling (ORD-02).
func TestClient_Connect_Timeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Host = "192.0.2.1" // TEST-NET, should timeout
	cfg.ConnectTimeout = 100 * time.Millisecond
	client := NewClient(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := client.Connect(ctx)
	if err == nil {
		t.Error("expected timeout error")
		client.Disconnect()
		return
	}

	// Should return connection timeout error
	if client.State() != broker.StateError {
		t.Errorf("expected state Error after timeout, got %v", client.State())
	}
}

// TestClient_Connect_AlreadyConnected tests idempotent connect.
func TestClient_Connect_AlreadyConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	// Manually set state to connected for testing
	client.state.Store(int32(broker.StateConnected))

	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		t.Errorf("expected no error for already connected, got %v", err)
	}
}

// TestClient_Disconnect tests clean disconnect.
func TestClient_Disconnect(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	// Disconnect when not connected should be no-op
	err := client.Disconnect()
	if err != nil {
		t.Errorf("expected no error disconnecting non-connected client, got %v", err)
	}

	if client.State() != broker.StateDisconnected {
		t.Error("expected state to remain Disconnected")
	}
}

// TestClient_GetAccountSummary_NotConnected tests account query when not connected.
func TestClient_GetAccountSummary_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.GetAccountSummary(ctx)

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_SubscribeMarketData_NotConnected tests subscription when not connected.
func TestClient_SubscribeMarketData_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.SubscribeMarketData(ctx, "MES")

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_PlaceOrder_NotConnected tests order when not connected (ORD-02).
func TestClient_PlaceOrder_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.PlaceOrder(ctx, types.OrderIntent{
		Symbol: "MES",
	})

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_CancelOrder_NotConnected tests cancel when not connected.
func TestClient_CancelOrder_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	err := client.CancelOrder(ctx, "order-123")

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_GetOpenOrders_NotConnected tests orders query when not connected.
func TestClient_GetOpenOrders_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.GetOpenOrders(ctx)

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_GetPositions_NotConnected tests positions query when not connected (REC-01).
func TestClient_GetPositions_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.GetPositions(ctx)

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_GetPosition_NotConnected tests single position query when not connected.
func TestClient_GetPosition_NotConnected(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx := context.Background()
	_, err := client.GetPosition(ctx, "MES")

	if err != broker.ErrNotConnected {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

// TestClient_UnsubscribeMarketData tests unsubscribe.
func TestClient_UnsubscribeMarketData(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	// Unsubscribe non-existent should be no-op
	err := client.UnsubscribeMarketData("MES")
	if err != nil {
		t.Errorf("expected no error for non-existent subscription, got %v", err)
	}
}

// TestClient_Shutdown tests graceful shutdown (SHUT-01).
func TestClient_Shutdown(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := client.Shutdown(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("expected no error on shutdown, got %v", err)
	}

	// SHUT-01: graceful shutdown < 15s
	if duration > 15*time.Second {
		t.Errorf("shutdown took too long: %v", duration)
	}

	if client.State() != broker.StateDisconnected {
		t.Error("expected state Disconnected after shutdown")
	}
}

// TestClient_BuildStartAPIMessage tests message construction.
func TestClient_BuildStartAPIMessage(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	msg := client.buildStartAPIMessage(1)

	// Message should have size prefix (4 bytes) + content
	if len(msg) < 4 {
		t.Error("message too short")
	}

	// Size prefix should be big-endian
	size := int(msg[0])<<24 | int(msg[1])<<16 | int(msg[2])<<8 | int(msg[3])
	expectedContentLen := len(msg) - 4
	if size != expectedContentLen {
		t.Errorf("size prefix %d does not match content length %d", size, expectedContentLen)
	}
}

// TestClient_RateLimiter tests rate limiting is configured (BC-03).
func TestClient_RateLimiter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRequestsPerSecond = 45
	client := NewClient(cfg, nil)

	// Verify limiter is configured
	if client.limiter == nil {
		t.Error("expected rate limiter to be configured")
	}

	// Limiter should allow burst
	for i := 0; i < 45; i++ {
		if !client.limiter.Allow() {
			t.Errorf("expected limiter to allow request %d", i)
		}
	}

	// Next request should be rate limited
	if client.limiter.Allow() {
		t.Error("expected limiter to deny request after burst")
	}
}

// TestClient_NextReqID tests request ID generation.
func TestClient_NextReqID(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClient(cfg, nil)

	id1 := client.nextReqID.Add(1)
	id2 := client.nextReqID.Add(1)

	if id2 <= id1 {
		t.Error("expected request IDs to be monotonically increasing")
	}
}

