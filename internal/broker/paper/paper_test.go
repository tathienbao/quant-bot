package paper

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestBroker_Connect(t *testing.T) {
	b := NewBroker(DefaultConfig(), nil)

	if err := b.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	if !b.IsConnected() {
		t.Error("expected connected state")
	}

	if b.State() != broker.StateConnected {
		t.Errorf("State() = %v, want connected", b.State())
	}
}

func TestBroker_GetAccountSummary(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialEquity = decimal.NewFromInt(10000)
	b := NewBroker(cfg, nil)
	b.Connect(context.Background())

	summary, err := b.GetAccountSummary(context.Background())
	if err != nil {
		t.Fatalf("GetAccountSummary() error = %v", err)
	}

	if summary.AccountID != "PAPER" {
		t.Errorf("AccountID = %s, want PAPER", summary.AccountID)
	}
	if !summary.NetLiquidation.Equal(cfg.InitialEquity) {
		t.Errorf("NetLiquidation = %s, want %s", summary.NetLiquidation, cfg.InitialEquity)
	}
}

func TestBroker_SubscribeMarketData(t *testing.T) {
	b := NewBroker(DefaultConfig(), nil)
	b.Connect(context.Background())

	ch, err := b.SubscribeMarketData(context.Background(), "MES")
	if err != nil {
		t.Fatalf("SubscribeMarketData() error = %v", err)
	}

	if ch == nil {
		t.Error("expected non-nil channel")
	}

	// Unsubscribe
	if err := b.UnsubscribeMarketData("MES"); err != nil {
		t.Errorf("UnsubscribeMarketData() error = %v", err)
	}
}

func TestBroker_SimulateMarketData(t *testing.T) {
	b := NewBroker(DefaultConfig(), nil)
	b.Connect(context.Background())

	ch, _ := b.SubscribeMarketData(context.Background(), "MES")

	// Simulate event
	event := types.MarketEvent{
		Timestamp: time.Now(),
		Symbol:    "MES",
		Open:      decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5010),
		Low:       decimal.NewFromInt(4990),
		Close:     decimal.NewFromInt(5005),
	}

	b.SimulateMarketData(event)

	// Should receive event
	select {
	case received := <-ch:
		if !received.Close.Equal(event.Close) {
			t.Errorf("Close = %s, want %s", received.Close, event.Close)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for market data")
	}
}

func TestBroker_PlaceOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FillDelay = 10 * time.Millisecond
	b := NewBroker(cfg, nil)
	b.Connect(context.Background())

	// Set price
	b.SimulateMarketData(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	intent := types.OrderIntent{
		ClientOrderID: "test-order-1",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		EntryPrice:    decimal.NewFromInt(5000),
	}

	result, err := b.PlaceOrder(context.Background(), intent)
	if err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	if result.ClientOrderID != intent.ClientOrderID {
		t.Errorf("ClientOrderID = %s, want %s", result.ClientOrderID, intent.ClientOrderID)
	}
	if result.Status != broker.OrderStatusSubmitted {
		t.Errorf("Status = %s, want submitted", result.Status)
	}

	// Wait for fill
	time.Sleep(50 * time.Millisecond)

	// Check position
	pos, err := b.GetPosition(context.Background(), "MES")
	if err != nil {
		t.Fatalf("GetPosition() error = %v", err)
	}

	if pos == nil {
		t.Fatal("expected position after fill")
	}
	if pos.Contracts != 1 {
		t.Errorf("Contracts = %d, want 1", pos.Contracts)
	}
	if pos.Side != types.SideLong {
		t.Errorf("Side = %v, want Long", pos.Side)
	}
}

func TestBroker_ClosePosition(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FillDelay = 10 * time.Millisecond
	cfg.InitialEquity = decimal.NewFromInt(10000)
	b := NewBroker(cfg, nil)
	b.Connect(context.Background())

	// Set initial price
	b.SimulateMarketData(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open position
	openIntent := types.OrderIntent{
		ClientOrderID: "open-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     2,
		EntryPrice:    decimal.NewFromInt(5000),
	}
	b.PlaceOrder(context.Background(), openIntent)
	time.Sleep(50 * time.Millisecond)

	// Update price (profit scenario)
	b.SimulateMarketData(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5010), // +10 points = +$25 per contract
	})

	// Close position
	closeIntent := types.OrderIntent{
		ClientOrderID: "close-order",
		Symbol:        "MES",
		Side:          types.SideShort, // Opposite side to close
		Contracts:     2,
		EntryPrice:    decimal.NewFromInt(5010),
	}
	b.PlaceOrder(context.Background(), closeIntent)
	time.Sleep(50 * time.Millisecond)

	// Check position closed
	pos, _ := b.GetPosition(context.Background(), "MES")
	if pos != nil {
		t.Errorf("expected nil position after close, got %+v", pos)
	}

	// Check equity increased
	equity := b.GetEquity()
	if equity.LessThanOrEqual(cfg.InitialEquity) {
		t.Errorf("Equity = %s, expected > %s", equity, cfg.InitialEquity)
	}
}

func TestBroker_GetOpenOrders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FillDelay = 1 * time.Second // Long delay to keep order open
	b := NewBroker(cfg, nil)
	b.Connect(context.Background())

	intent := types.OrderIntent{
		ClientOrderID: "test-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		EntryPrice:    decimal.NewFromInt(5000),
	}

	b.PlaceOrder(context.Background(), intent)

	orders, err := b.GetOpenOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOpenOrders() error = %v", err)
	}

	if len(orders) != 1 {
		t.Errorf("len(orders) = %d, want 1", len(orders))
	}
}

func TestBroker_CancelOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FillDelay = 1 * time.Second
	b := NewBroker(cfg, nil)
	b.Connect(context.Background())

	intent := types.OrderIntent{
		ClientOrderID: "cancel-test",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		EntryPrice:    decimal.NewFromInt(5000),
	}

	result, _ := b.PlaceOrder(context.Background(), intent)

	// Cancel order
	if err := b.CancelOrder(context.Background(), result.OrderID); err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}

	// Verify cancelled
	orders, _ := b.GetOpenOrders(context.Background())
	if len(orders) != 0 {
		t.Errorf("expected no open orders after cancel, got %d", len(orders))
	}
}

func TestBroker_Disconnect(t *testing.T) {
	b := NewBroker(DefaultConfig(), nil)
	b.Connect(context.Background())

	if err := b.Disconnect(); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}

	if b.IsConnected() {
		t.Error("expected disconnected state")
	}
}

func TestBroker_NotConnected(t *testing.T) {
	b := NewBroker(DefaultConfig(), nil)

	// Should fail when not connected
	_, err := b.PlaceOrder(context.Background(), types.OrderIntent{})
	if err != broker.ErrNotConnected {
		t.Errorf("PlaceOrder() error = %v, want ErrNotConnected", err)
	}
}
