package risk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestEngine_NewEngine(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	if engine.IsInSafeMode() {
		t.Error("New engine should not be in safe mode")
	}

	snapshot := engine.GetSnapshot()
	if !snapshot.Equity.Equal(decimal.RequireFromString("10000")) {
		t.Errorf("Initial equity = %s, want 10000", snapshot.Equity)
	}

	if !snapshot.Drawdown.IsZero() {
		t.Errorf("Initial drawdown = %s, want 0", snapshot.Drawdown)
	}
}

func TestEngine_ValidateAndSize_Success(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	signal := types.Signal{
		ID:        "sig-001",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
		ATR:    decimal.RequireFromString("2.5"),
	}

	intent, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if intent == nil {
		t.Fatal("Expected order intent, got nil")
	}

	if intent.Symbol != "MES" {
		t.Errorf("Symbol = %s, want MES", intent.Symbol)
	}

	if intent.Side != types.SideLong {
		t.Errorf("Side = %v, want LONG", intent.Side)
	}

	if intent.Contracts <= 0 {
		t.Errorf("Contracts = %d, want > 0", intent.Contracts)
	}

	if intent.StopLoss.IsZero() {
		t.Error("StopLoss should not be zero")
	}

	if intent.TakeProfit.IsZero() {
		t.Error("TakeProfit should not be zero")
	}

	// For long, stop loss should be below entry
	if intent.StopLoss.GreaterThanOrEqual(intent.EntryPrice) {
		t.Errorf("StopLoss (%s) should be < EntryPrice (%s) for long",
			intent.StopLoss, intent.EntryPrice)
	}

	// For long, take profit should be above entry
	if intent.TakeProfit.LessThanOrEqual(intent.EntryPrice) {
		t.Errorf("TakeProfit (%s) should be > EntryPrice (%s) for long",
			intent.TakeProfit, intent.EntryPrice)
	}
}

func TestEngine_ValidateAndSize_ShortPosition(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	signal := types.Signal{
		ID:        "sig-002",
		Symbol:    "MES",
		Direction: types.SideShort,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
		ATR:    decimal.RequireFromString("2.5"),
	}

	intent, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// For short, stop loss should be above entry
	if intent.StopLoss.LessThanOrEqual(intent.EntryPrice) {
		t.Errorf("StopLoss (%s) should be > EntryPrice (%s) for short",
			intent.StopLoss, intent.EntryPrice)
	}

	// For short, take profit should be below entry
	if intent.TakeProfit.GreaterThanOrEqual(intent.EntryPrice) {
		t.Errorf("TakeProfit (%s) should be < EntryPrice (%s) for short",
			intent.TakeProfit, intent.EntryPrice)
	}
}

func TestEngine_ValidateAndSize_SafeMode(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// Enter safe mode
	engine.EnterSafeMode("test")

	signal := types.Signal{
		ID:        "sig-003",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
	}

	_, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if !errors.Is(err, types.ErrKillSwitchActive) {
		t.Errorf("Expected ErrKillSwitchActive, got: %v", err)
	}
}

func TestEngine_ValidateAndSize_DrawdownTriggersKillSwitch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxGlobalDrawdownPct = decimal.RequireFromString("0.20") // 20%

	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// Simulate drawdown: 10000 -> 7900 = 21% drawdown
	engine.UpdateEquity(decimal.RequireFromString("7900"))

	signal := types.Signal{
		ID:        "sig-004",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
	}

	_, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	// When drawdown exceeds limit, engine enters safe mode and returns ErrKillSwitchActive
	if !errors.Is(err, types.ErrKillSwitchActive) {
		t.Errorf("Expected ErrKillSwitchActive, got: %v", err)
	}

	// Should be in safe mode now
	if !engine.IsInSafeMode() {
		t.Error("Engine should be in safe mode after max drawdown")
	}
}

func TestEngine_ValidateAndSize_InsufficientEquity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RiskPerTradePct = decimal.RequireFromString("0.001") // 0.1% = very small risk

	engine := NewEngine(cfg, decimal.RequireFromString("100"), nil) // Very small account

	signal := types.Signal{
		ID:        "sig-005",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 100, // Large stop
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
	}

	_, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if !errors.Is(err, types.ErrInsufficientEquity) {
		t.Errorf("Expected ErrInsufficientEquity, got: %v", err)
	}
}

func TestEngine_ValidateAndSize_InvalidSymbol(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	signal := types.Signal{
		ID:        "sig-006",
		Symbol:    "INVALID",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "INVALID",
		Close:  decimal.RequireFromString("5000"),
	}

	_, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if !errors.Is(err, types.ErrInvalidSymbol) {
		t.Errorf("Expected ErrInvalidSymbol, got: %v", err)
	}
}

func TestEngine_ValidateAndSize_ContextCancelled(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	signal := types.Signal{
		ID:        "sig-007",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
	}

	_, err := engine.ValidateAndSize(ctx, signal, marketEvent)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestEngine_UpdateEquity(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// Equity goes up
	engine.UpdateEquity(decimal.RequireFromString("11000"))
	snapshot := engine.GetSnapshot()
	if !snapshot.Equity.Equal(decimal.RequireFromString("11000")) {
		t.Errorf("Equity = %s, want 11000", snapshot.Equity)
	}
	if !snapshot.HighWaterMark.Equal(decimal.RequireFromString("11000")) {
		t.Errorf("HighWaterMark = %s, want 11000", snapshot.HighWaterMark)
	}

	// Equity goes down
	engine.UpdateEquity(decimal.RequireFromString("10000"))
	snapshot = engine.GetSnapshot()
	if !snapshot.Equity.Equal(decimal.RequireFromString("10000")) {
		t.Errorf("Equity = %s, want 10000", snapshot.Equity)
	}
	if !snapshot.HighWaterMark.Equal(decimal.RequireFromString("11000")) {
		t.Errorf("HighWaterMark = %s, want 11000 (should not decrease)", snapshot.HighWaterMark)
	}
}

func TestEngine_UpdateEquity_TriggersKillSwitch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxGlobalDrawdownPct = decimal.RequireFromString("0.10") // 10%

	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// Create peak
	engine.UpdateEquity(decimal.RequireFromString("11000"))

	// 11% drawdown should trigger kill switch
	engine.UpdateEquity(decimal.RequireFromString("9790"))

	if !engine.IsInSafeMode() {
		t.Error("Engine should be in safe mode after 11% drawdown (limit is 10%)")
	}
}

func TestEngine_SafeMode(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// Initially not in safe mode
	if engine.IsInSafeMode() {
		t.Error("Should not start in safe mode")
	}

	// Enter safe mode
	engine.EnterSafeMode("test reason")
	if !engine.IsInSafeMode() {
		t.Error("Should be in safe mode after EnterSafeMode")
	}

	// Exit safe mode
	engine.ExitSafeMode()
	if engine.IsInSafeMode() {
		t.Error("Should not be in safe mode after ExitSafeMode")
	}
}

func TestEngine_Position(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	// No position initially
	_, ok := engine.GetPosition("MES")
	if ok {
		t.Error("Should not have position initially")
	}

	// Add position
	pos := &types.Position{
		ID:         "pos-001",
		Symbol:     "MES",
		Side:       types.SideLong,
		Contracts:  2,
		EntryPrice: decimal.RequireFromString("5000"),
		EntryTime:  time.Now(),
	}
	engine.UpdatePosition(pos)

	// Get position
	gotPos, ok := engine.GetPosition("MES")
	if !ok {
		t.Fatal("Should have position after update")
	}
	if gotPos.Contracts != 2 {
		t.Errorf("Contracts = %d, want 2", gotPos.Contracts)
	}

	// Close position (0 contracts)
	pos.Contracts = 0
	engine.UpdatePosition(pos)

	_, ok = engine.GetPosition("MES")
	if ok {
		t.Error("Should not have position after closing")
	}
}

func TestEngine_ExposureLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxExposurePerSymbolPct = decimal.RequireFromString("0.5")  // 50%
	cfg.MaxTotalExposurePct = decimal.RequireFromString("1.0")      // 100%
	cfg.RiskPerTradePct = decimal.RequireFromString("0.05")         // 5% to get larger positions

	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	signal := types.Signal{
		ID:        "sig-008",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 2, // Very tight stop to maximize position size
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
		ATR:    decimal.RequireFromString("2.5"),
	}

	// This might fail due to exposure limits depending on calculated size
	intent, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)

	// If it succeeds, verify we got an order
	if err == nil && intent != nil {
		if intent.Contracts <= 0 {
			t.Error("Expected positive contracts")
		}
	}

	// If it fails with exposure limit, that's also valid behavior
	if err != nil && !errors.Is(err, types.ErrExposureLimitExceeded) && !errors.Is(err, types.ErrInsufficientEquity) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestEngine_Shutdown(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := engine.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestEngine_ATRBasedStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StopLossATRMultiple = decimal.RequireFromString("2.0")

	engine := NewEngine(cfg, decimal.RequireFromString("10000"), nil)

	signal := types.Signal{
		ID:        "sig-009",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 0, // No stop specified, should use ATR
	}

	marketEvent := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.RequireFromString("5000"),
		ATR:    decimal.RequireFromString("2.5"), // 2.5 points ATR
	}

	intent, err := engine.ValidateAndSize(context.Background(), signal, marketEvent)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// ATR = 2.5, multiplier = 2.0, so stop = 5 points
	// Tick size = 0.25, so stop = 20 ticks
	// For MES, each point = 4 ticks
	// Stop should be entry - 5 points = 4995

	expectedStopApprox := decimal.RequireFromString("4995")
	diff := intent.StopLoss.Sub(expectedStopApprox).Abs()
	if diff.GreaterThan(decimal.RequireFromString("0.5")) {
		t.Errorf("StopLoss = %s, expected approximately %s", intent.StopLoss, expectedStopApprox)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.MaxGlobalDrawdownPct.Equal(decimal.RequireFromString("0.20")) {
		t.Errorf("MaxGlobalDrawdownPct = %s, want 0.20", cfg.MaxGlobalDrawdownPct)
	}

	if !cfg.RiskPerTradePct.Equal(decimal.RequireFromString("0.01")) {
		t.Errorf("RiskPerTradePct = %s, want 0.01", cfg.RiskPerTradePct)
	}
}
