package observer

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// TestNewCalculator tests calculator constructor.
func TestNewCalculator(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	calc := NewCalculator(cfg)

	if calc == nil {
		t.Fatal("expected calculator to be created")
	}

	if calc.cfg.ATRPeriod != 14 {
		t.Errorf("expected ATR period 14, got %d", calc.cfg.ATRPeriod)
	}

	if calc.cfg.StdDevPeriod != 20 {
		t.Errorf("expected StdDev period 20, got %d", calc.cfg.StdDevPeriod)
	}
}

// TestCalculator_OnBar tests indicator update.
func TestCalculator_OnBar(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.ATRPeriod = 3 // Shorter for testing
	calc := NewCalculator(cfg)

	events := []types.MarketEvent{
		{Timestamp: time.Now(), Symbol: "MES", High: decimal.NewFromInt(5010), Low: decimal.NewFromInt(4990), Close: decimal.NewFromInt(5000)},
		{Timestamp: time.Now(), Symbol: "MES", High: decimal.NewFromInt(5020), Low: decimal.NewFromInt(4980), Close: decimal.NewFromInt(5010)},
		{Timestamp: time.Now(), Symbol: "MES", High: decimal.NewFromInt(5030), Low: decimal.NewFromInt(5000), Close: decimal.NewFromInt(5020)},
	}

	var lastEvent types.MarketEvent
	for _, event := range events {
		lastEvent = calc.OnBar(event)
	}

	// ATR should be calculated after warmup
	if lastEvent.ATR.IsZero() {
		t.Error("expected ATR to be calculated")
	}
}

// TestCalculator_Ready tests warmup check.
func TestCalculator_Ready(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.ATRPeriod = 3
	cfg.StdDevPeriod = 3
	calc := NewCalculator(cfg)

	// Not ready initially
	if calc.Ready() {
		t.Error("expected calculator to not be ready initially")
	}

	// Feed warmup bars
	for i := 0; i < 5; i++ {
		event := types.MarketEvent{
			High:  decimal.NewFromInt(int64(5000 + i*10)),
			Low:   decimal.NewFromInt(int64(4990 + i*10)),
			Close: decimal.NewFromInt(int64(5000 + i*10)),
		}
		calc.OnBar(event)
	}

	// Should be ready after warmup
	if !calc.Ready() {
		t.Error("expected calculator to be ready after warmup")
	}
}

// TestCalculator_Reset tests state reset.
func TestCalculator_Reset(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.ATRPeriod = 3
	cfg.StdDevPeriod = 3
	calc := NewCalculator(cfg)

	// Feed some bars
	for i := 0; i < 5; i++ {
		event := types.MarketEvent{
			High:  decimal.NewFromInt(int64(5000 + i*10)),
			Low:   decimal.NewFromInt(int64(4990 + i*10)),
			Close: decimal.NewFromInt(int64(5000 + i*10)),
		}
		calc.OnBar(event)
	}

	// Reset
	calc.Reset()

	// Should not be ready after reset
	if calc.Ready() {
		t.Error("expected calculator to not be ready after reset")
	}

	// ATR should be zero
	if !calc.CurrentATR().IsZero() {
		t.Error("expected ATR to be zero after reset")
	}
}

// TestCalculator_CurrentATR tests ATR retrieval.
func TestCalculator_CurrentATR(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.ATRPeriod = 2
	calc := NewCalculator(cfg)

	// Initially zero
	if !calc.CurrentATR().IsZero() {
		t.Error("expected initial ATR to be zero")
	}

	// Feed bars
	events := []types.MarketEvent{
		{High: decimal.NewFromInt(5010), Low: decimal.NewFromInt(4990), Close: decimal.NewFromInt(5000)},
		{High: decimal.NewFromInt(5030), Low: decimal.NewFromInt(4980), Close: decimal.NewFromInt(5020)},
		{High: decimal.NewFromInt(5050), Low: decimal.NewFromInt(5000), Close: decimal.NewFromInt(5040)},
	}

	for _, event := range events {
		calc.OnBar(event)
	}

	atr := calc.CurrentATR()
	if atr.IsZero() {
		t.Error("expected ATR to be calculated")
	}

	// ATR should be positive
	if atr.LessThanOrEqual(decimal.Zero) {
		t.Error("expected ATR to be positive")
	}
}

// TestCalculator_CurrentATR_ZeroData tests ATR = 0 handling (SL-05).
func TestCalculator_CurrentATR_ZeroData(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	calc := NewCalculator(cfg)

	// Before any data, ATR should be zero
	atr := calc.CurrentATR()
	if !atr.IsZero() {
		t.Error("expected ATR to be zero without data (SL-05)")
	}

	// Feed identical bars (no range)
	for i := 0; i < 20; i++ {
		event := types.MarketEvent{
			High:  decimal.NewFromInt(5000),
			Low:   decimal.NewFromInt(5000),
			Close: decimal.NewFromInt(5000),
		}
		calc.OnBar(event)
	}

	// ATR should be zero when there's no range
	atr = calc.CurrentATR()
	if !atr.IsZero() {
		t.Errorf("expected ATR to be zero for zero-range bars, got %s", atr.String())
	}
}

// TestCalculator_CurrentStdDev tests StdDev retrieval.
func TestCalculator_CurrentStdDev(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.StdDevPeriod = 3
	calc := NewCalculator(cfg)

	// Feed bars with variance
	events := []types.MarketEvent{
		{Close: decimal.NewFromInt(5000)},
		{Close: decimal.NewFromInt(5010)},
		{Close: decimal.NewFromInt(4990)},
		{Close: decimal.NewFromInt(5020)},
	}

	for _, event := range events {
		calc.OnBar(event)
	}

	stddev := calc.CurrentStdDev()
	if stddev.IsZero() {
		t.Error("expected StdDev to be calculated")
	}

	// StdDev should be positive for varying data
	if stddev.LessThanOrEqual(decimal.Zero) {
		t.Error("expected StdDev to be positive")
	}
}

// TestCalculator_CurrentSMA tests SMA retrieval.
func TestCalculator_CurrentSMA(t *testing.T) {
	cfg := DefaultCalculatorConfig()
	cfg.SMAPeriod = 3
	calc := NewCalculator(cfg)

	// Feed bars
	events := []types.MarketEvent{
		{Close: decimal.NewFromInt(100)},
		{Close: decimal.NewFromInt(200)},
		{Close: decimal.NewFromInt(300)},
	}

	for _, event := range events {
		calc.OnBar(event)
	}

	sma := calc.CurrentSMA()
	// SMA of 100, 200, 300 = 200
	expected := decimal.NewFromInt(200)
	if !sma.Equal(expected) {
		t.Errorf("expected SMA=%s, got %s", expected.String(), sma.String())
	}
}

// TestDefaultCalculatorConfig tests default configuration.
func TestDefaultCalculatorConfig(t *testing.T) {
	cfg := DefaultCalculatorConfig()

	if cfg.ATRPeriod != 14 {
		t.Errorf("expected ATR period 14, got %d", cfg.ATRPeriod)
	}

	if cfg.StdDevPeriod != 20 {
		t.Errorf("expected StdDev period 20, got %d", cfg.StdDevPeriod)
	}

	if cfg.SMAPeriod != 20 {
		t.Errorf("expected SMA period 20, got %d", cfg.SMAPeriod)
	}
}
