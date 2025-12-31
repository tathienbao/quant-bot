package execution

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestSimulatedExecutor_PlaceOrder_Long(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    1,
		CommissionPerSide: decimal.RequireFromString("0.62"),
	})

	// Set market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	order := types.OrderIntent{
		ClientOrderID: "test-order-1",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		StopLoss:      decimal.NewFromInt(4990),
		TakeProfit:    decimal.NewFromInt(5020),
	}

	result, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	if result.Status != types.OrderStatusFilled {
		t.Errorf("Status = %v, want FILLED", result.Status)
	}

	// Check fill price includes slippage (buy higher)
	expectedFill := decimal.NewFromInt(5000).Add(decimal.RequireFromString("0.25"))
	if !result.AvgFillPrice.Equal(expectedFill) {
		t.Errorf("AvgFillPrice = %s, want %s", result.AvgFillPrice, expectedFill)
	}

	// Check position created
	pos, err := exec.GetPosition(context.Background(), "MES")
	if err != nil {
		t.Fatalf("GetPosition failed: %v", err)
	}
	if pos == nil {
		t.Fatal("Expected position to be created")
	}
	if pos.Side != types.SideLong {
		t.Errorf("Position side = %v, want LONG", pos.Side)
	}
}

func TestSimulatedExecutor_PlaceOrder_Short(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    1,
		CommissionPerSide: decimal.RequireFromString("0.62"),
	})

	// Set market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	order := types.OrderIntent{
		ClientOrderID: "test-order-1",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}

	result, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Check fill price includes slippage (sell lower)
	expectedFill := decimal.NewFromInt(5000).Sub(decimal.RequireFromString("0.25"))
	if !result.AvgFillPrice.Equal(expectedFill) {
		t.Errorf("AvgFillPrice = %s, want %s", result.AvgFillPrice, expectedFill)
	}
}

func TestSimulatedExecutor_DuplicateOrder(t *testing.T) {
	exec := NewSimulatedExecutor(DefaultSimulatedConfig())

	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	order := types.OrderIntent{
		ClientOrderID: "duplicate-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}

	// First order should succeed
	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("First order failed: %v", err)
	}

	// Close the position first
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-order",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	// Duplicate order should fail
	_, err = exec.PlaceOrder(context.Background(), order)
	if err == nil {
		t.Error("Expected duplicate order to fail")
	}
}

func TestSimulatedExecutor_StopLoss_Long(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    1,
		CommissionPerSide: decimal.RequireFromString("0.62"),
	})

	// Set initial market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5005),
		Low:       decimal.NewFromInt(4995),
	})

	// Place long order
	order := types.OrderIntent{
		ClientOrderID: "long-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		StopLoss:      decimal.NewFromInt(4990),
	}

	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Market drops to hit stop loss
	fills := exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(4985),
		High:      decimal.NewFromInt(4995),
		Low:       decimal.NewFromInt(4980), // Below stop
	})

	if len(fills) != 1 {
		t.Fatalf("Expected 1 fill from stop loss, got %d", len(fills))
	}

	// Position should be closed
	pos, _ := exec.GetPosition(context.Background(), "MES")
	if pos != nil {
		t.Error("Position should be closed after stop loss")
	}

	// Check trade recorded
	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	// PL should be negative (losing trade)
	if trades[0].NetPL.GreaterThanOrEqual(decimal.Zero) {
		t.Errorf("Expected negative PL on stop loss, got %s", trades[0].NetPL)
	}
}

func TestSimulatedExecutor_TakeProfit_Long(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0, // No slippage for simpler test
		CommissionPerSide: decimal.Zero,
	})

	// Set initial market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5005),
		Low:       decimal.NewFromInt(4995),
	})

	// Place long order
	order := types.OrderIntent{
		ClientOrderID: "long-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		TakeProfit:    decimal.NewFromInt(5020),
	}

	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Market rises to hit take profit
	fills := exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5025),
		High:      decimal.NewFromInt(5025), // Above TP
		Low:       decimal.NewFromInt(5010),
	})

	if len(fills) != 1 {
		t.Fatalf("Expected 1 fill from take profit, got %d", len(fills))
	}

	// Check trade recorded with positive PL
	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	// Entry at 5000, exit at 5020 = 20 points * $5/point = $100
	expectedPL := decimal.NewFromInt(100)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("GrossPL = %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

func TestSimulatedExecutor_StopLoss_Short(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    1,
		CommissionPerSide: decimal.Zero,
	})

	// Set initial market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5005),
		Low:       decimal.NewFromInt(4995),
	})

	// Place short order
	order := types.OrderIntent{
		ClientOrderID: "short-order",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
		StopLoss:      decimal.NewFromInt(5010),
	}

	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Market rises to hit stop loss
	fills := exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5015),
		High:      decimal.NewFromInt(5015), // Above stop
		Low:       decimal.NewFromInt(5005),
	})

	if len(fills) != 1 {
		t.Fatalf("Expected 1 fill from stop loss, got %d", len(fills))
	}

	// Position should be closed
	pos, _ := exec.GetPosition(context.Background(), "MES")
	if pos != nil {
		t.Error("Position should be closed after stop loss")
	}
}

func TestSimulatedExecutor_ClosePosition(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.RequireFromString("0.62"),
	})

	// Set market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	// Open long position
	openOrder := types.OrderIntent{
		ClientOrderID: "open-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, err := exec.PlaceOrder(context.Background(), openOrder)
	if err != nil {
		t.Fatalf("Open order failed: %v", err)
	}

	// Update market price higher
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5010),
		High:      decimal.NewFromInt(5015),
		Low:       decimal.NewFromInt(5005),
	})

	// Close position with opposite order
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-order",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	result, err := exec.PlaceOrder(context.Background(), closeOrder)
	if err != nil {
		t.Fatalf("Close order failed: %v", err)
	}

	if result.Status != types.OrderStatusFilled {
		t.Errorf("Status = %v, want FILLED", result.Status)
	}

	// Position should be closed
	pos, _ := exec.GetPosition(context.Background(), "MES")
	if pos != nil {
		t.Error("Position should be closed")
	}

	// Check trade recorded
	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	// Profit = 10 points * $5 = $50, minus $0.62 commission = $49.38
	expectedNet := decimal.RequireFromString("49.38")
	if !trades[0].NetPL.Equal(expectedNet) {
		t.Errorf("NetPL = %s, want %s", trades[0].NetPL, expectedNet)
	}
}

func TestSimulatedExecutor_Reset(t *testing.T) {
	exec := NewSimulatedExecutor(DefaultSimulatedConfig())

	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	// Place an order
	order := types.OrderIntent{
		ClientOrderID: "test-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	exec.Reset()

	// Everything should be cleared
	pos, _ := exec.GetPosition(context.Background(), "MES")
	if pos != nil {
		t.Error("Position should be nil after reset")
	}

	trades := exec.GetTrades()
	if len(trades) != 0 {
		t.Errorf("Trades should be empty after reset, got %d", len(trades))
	}
}

func TestSimulatedExecutor_UnrealizedPL(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	// Set market price
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5000),
	})

	// Open long position
	order := types.OrderIntent{
		ClientOrderID: "test-order",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     2,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Market moves up
	exec.UpdateMarket(types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Close:     decimal.NewFromInt(5010),
		High:      decimal.NewFromInt(5015),
		Low:       decimal.NewFromInt(5005),
	})

	pos, _ := exec.GetPosition(context.Background(), "MES")
	if pos == nil {
		t.Fatal("Expected position")
	}

	// Unrealized PL = 10 points * $5 * 2 contracts = $100
	expectedPL := decimal.NewFromInt(100)
	if !pos.UnrealizedPL.Equal(expectedPL) {
		t.Errorf("UnrealizedPL = %s, want %s", pos.UnrealizedPL, expectedPL)
	}
}
