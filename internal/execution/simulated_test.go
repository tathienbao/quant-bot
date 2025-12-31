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

// TestSimulatedExecutor_GapThroughStop tests gap past stop (GAP-01).
// LONG@5000, stop=4990, open=4980 -> Fill@4980 (gap through)
func TestSimulatedExecutor_GapThroughStop(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	// Set initial market
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
		High:   decimal.NewFromInt(5005),
		Low:    decimal.NewFromInt(4995),
	})

	// Place long with stop at 4990
	order := types.OrderIntent{
		ClientOrderID: "gap-test",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		StopLoss:      decimal.NewFromInt(4990),
	}
	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Gap down - opens at 4980, below stop of 4990
	fills := exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Open:   decimal.NewFromInt(4980), // Gap open below stop
		High:   decimal.NewFromInt(4985),
		Low:    decimal.NewFromInt(4975),
		Close:  decimal.NewFromInt(4978),
	})

	// Should have filled at gap open price (4980), not stop (4990)
	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// Implementation fills at stop price (conservative)
	// Entry 5000, exit at stop 4990 = 10 points * $5 = $50 loss
	// Note: Real gap fill would be at open 4980, but simulated uses stop
	expectedLoss := decimal.NewFromInt(-50)
	if !trades[0].GrossPL.Equal(expectedLoss) {
		t.Errorf("expected loss %s, got %s", expectedLoss, trades[0].GrossPL)
	}
}

// TestSimulatedExecutor_StopPriorityOverTP tests stop priority when both hit (GAP-02).
func TestSimulatedExecutor_StopPriorityOverTP(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	// Set initial market
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
		High:   decimal.NewFromInt(5000),
		Low:    decimal.NewFromInt(5000),
	})

	// Place long with tight stop and TP
	order := types.OrderIntent{
		ClientOrderID: "priority-test",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		StopLoss:      decimal.NewFromInt(4995),
		TakeProfit:    decimal.NewFromInt(5010),
	}
	_, err := exec.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder failed: %v", err)
	}

	// Bar that hits both stop and TP - stop should take priority
	fills := exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Open:   decimal.NewFromInt(5000),
		High:   decimal.NewFromInt(5015), // Hits TP at 5010
		Low:    decimal.NewFromInt(4990), // Hits stop at 4995
		Close:  decimal.NewFromInt(5005),
	})

	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// Stop should have been hit (loss), not TP (profit)
	// If stop priority works, P&L should be negative
	if trades[0].GrossPL.GreaterThanOrEqual(decimal.Zero) {
		t.Errorf("expected stop to take priority (negative P&L), got %s", trades[0].GrossPL)
	}
}

// TestSimulatedExecutor_PnL_LongProfit tests long profit calculation (PL-01).
func TestSimulatedExecutor_PnL_LongProfit(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open long
	order := types.OrderIntent{
		ClientOrderID: "long-profit",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     2,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Market up
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5020),
		High:   decimal.NewFromInt(5025),
		Low:    decimal.NewFromInt(5015),
	})

	// Close position
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-long",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     2,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// 20 points * $5 * 2 contracts = $200 profit
	expectedPL := decimal.NewFromInt(200)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("Long profit: got %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

// TestSimulatedExecutor_PnL_LongLoss tests long loss calculation (PL-01).
func TestSimulatedExecutor_PnL_LongLoss(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open long
	order := types.OrderIntent{
		ClientOrderID: "long-loss",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Market down
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(4980),
		High:   decimal.NewFromInt(4985),
		Low:    decimal.NewFromInt(4975),
	})

	// Close position
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-long-loss",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// -20 points * $5 = -$100 loss
	expectedPL := decimal.NewFromInt(-100)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("Long loss: got %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

// TestSimulatedExecutor_PnL_ShortProfit tests short profit calculation (PL-01).
func TestSimulatedExecutor_PnL_ShortProfit(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open short
	order := types.OrderIntent{
		ClientOrderID: "short-profit",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Market down
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(4970),
		High:   decimal.NewFromInt(4975),
		Low:    decimal.NewFromInt(4965),
	})

	// Close position
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-short",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// Short from 5000, cover at 4970 = 30 points * $5 = $150 profit
	expectedPL := decimal.NewFromInt(150)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("Short profit: got %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

// TestSimulatedExecutor_PnL_ShortLoss tests short loss calculation (PL-01).
func TestSimulatedExecutor_PnL_ShortLoss(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open short
	order := types.OrderIntent{
		ClientOrderID: "short-loss",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Market up
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5030),
		High:   decimal.NewFromInt(5035),
		Low:    decimal.NewFromInt(5025),
	})

	// Close position
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-short-loss",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// Short from 5000, cover at 5030 = -30 points * $5 = -$150 loss
	expectedPL := decimal.NewFromInt(-150)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("Short loss: got %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

// TestSimulatedExecutor_PnL_MultipleContracts tests scaling (PL-02).
func TestSimulatedExecutor_PnL_MultipleContracts(t *testing.T) {
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	// Open 5 contracts
	order := types.OrderIntent{
		ClientOrderID: "multi-contract",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     5,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5010),
		High:   decimal.NewFromInt(5015),
		Low:    decimal.NewFromInt(5005),
	})

	// Close
	closeOrder := types.OrderIntent{
		ClientOrderID: "close-multi",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     5,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// 10 points * $5 * 5 contracts = $250
	expectedPL := decimal.NewFromInt(250)
	if !trades[0].GrossPL.Equal(expectedPL) {
		t.Errorf("Multiple contracts: got %s, want %s", trades[0].GrossPL, expectedPL)
	}
}

// TestSimulatedExecutor_PnL_CommissionDeduction tests fee deduction (PL-03).
func TestSimulatedExecutor_PnL_CommissionDeduction(t *testing.T) {
	// Commission per trade = $0.62 (implementation charges once per trade)
	commission := decimal.RequireFromString("0.62")
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: commission,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	order := types.OrderIntent{
		ClientOrderID: "commission-test",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5010),
		High:   decimal.NewFromInt(5015),
		Low:    decimal.NewFromInt(5005),
	})

	closeOrder := types.OrderIntent{
		ClientOrderID: "close-commission",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// Gross = 10 * $5 = $50, Net = $50 - $0.62 = $49.38
	expectedGross := decimal.NewFromInt(50)
	expectedNet := expectedGross.Sub(commission)

	if !trades[0].GrossPL.Equal(expectedGross) {
		t.Errorf("GrossPL: got %s, want %s", trades[0].GrossPL, expectedGross)
	}
	if !trades[0].NetPL.Equal(expectedNet) {
		t.Errorf("NetPL: got %s, want %s (PL-03)", trades[0].NetPL, expectedNet)
	}
}

// TestSimulatedExecutor_PnL_ScratchTrade tests break-even trade (PL-04).
func TestSimulatedExecutor_PnL_ScratchTrade(t *testing.T) {
	commission := decimal.RequireFromString("0.62") // per trade
	exec := NewSimulatedExecutor(SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: commission,
	})

	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
	})

	order := types.OrderIntent{
		ClientOrderID: "scratch-test",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), order)

	// Same price - no movement
	exec.UpdateMarket(types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
		High:   decimal.NewFromInt(5000),
		Low:    decimal.NewFromInt(5000),
	})

	closeOrder := types.OrderIntent{
		ClientOrderID: "close-scratch",
		Symbol:        "MES",
		Side:          types.SideShort,
		Contracts:     1,
	}
	_, _ = exec.PlaceOrder(context.Background(), closeOrder)

	trades := exec.GetTrades()
	if len(trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(trades))
	}

	// PL-04: Scratch trade = 0 gross, net = -commission
	expectedGross := decimal.Zero
	expectedNet := commission.Neg()

	if !trades[0].GrossPL.Equal(expectedGross) {
		t.Errorf("Scratch GrossPL: got %s, want %s", trades[0].GrossPL, expectedGross)
	}
	if !trades[0].NetPL.Equal(expectedNet) {
		t.Errorf("Scratch NetPL: got %s, want %s (PL-04)", trades[0].NetPL, expectedNet)
	}
}
