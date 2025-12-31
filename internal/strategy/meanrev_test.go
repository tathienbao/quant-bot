package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestMeanReversion_NotReadyUntilEnoughBars(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 5
	cfg.StdDevPeriod = 5
	strategy := NewMeanReversion(cfg)

	// Feed only 3 bars
	for i := 0; i < 3; i++ {
		event := createMREvent(decimal.NewFromInt(100))
		signals := strategy.OnMarketEvent(context.Background(), event)
		if len(signals) > 0 {
			t.Error("Should not generate signals before enough bars")
		}
	}
}

func TestMeanReversion_LongSignalBelowLowerBand(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars with mean around 100
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// Mean = 100, StdDev ≈ 1.63
	// Lower band = 100 - (2 * 1.63) ≈ 96.74

	// Price drops well below lower band
	event := createMREvent(decimal.NewFromInt(90))
	event.ATR = decimal.NewFromInt(2)
	signals := strategy.OnMarketEvent(context.Background(), event)

	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	if signals[0].Direction != types.SideLong {
		t.Errorf("Direction = %v, want LONG (mean reversion up)", signals[0].Direction)
	}

	if signals[0].StrategyName != "meanrev" {
		t.Errorf("StrategyName = %s, want meanrev", signals[0].StrategyName)
	}
}

func TestMeanReversion_ShortSignalAboveUpperBand(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars with mean around 100
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// Mean = 100, StdDev ≈ 1.63
	// Upper band = 100 + (2 * 1.63) ≈ 103.26

	// Price spikes above upper band
	event := createMREvent(decimal.NewFromInt(110))
	event.ATR = decimal.NewFromInt(2)
	signals := strategy.OnMarketEvent(context.Background(), event)

	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	if signals[0].Direction != types.SideShort {
		t.Errorf("Direction = %v, want SHORT (mean reversion down)", signals[0].Direction)
	}
}

func TestMeanReversion_NoSignalWithinBands(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// Price stays within bands
	event := createMREvent(decimal.NewFromInt(101))
	event.ATR = decimal.NewFromInt(2)
	signals := strategy.OnMarketEvent(context.Background(), event)

	if len(signals) != 0 {
		t.Errorf("Expected 0 signals within bands, got %d", len(signals))
	}
}

func TestMeanReversion_NoRepeatedSignals(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// First signal below lower band
	event1 := createMREvent(decimal.NewFromInt(90))
	event1.ATR = decimal.NewFromInt(2)
	signals1 := strategy.OnMarketEvent(context.Background(), event1)
	if len(signals1) != 1 {
		t.Fatalf("Expected 1 signal on first touch, got %d", len(signals1))
	}

	// Still below - should NOT signal again
	event2 := createMREvent(decimal.NewFromInt(88))
	event2.ATR = decimal.NewFromInt(2)
	signals2 := strategy.OnMarketEvent(context.Background(), event2)
	if len(signals2) != 0 {
		t.Errorf("Should not repeat signal, got %d", len(signals2))
	}
}

func TestMeanReversion_ResetAllowsNewSignal(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars to build indicators (need 3 for ready, then 1 more to signal)
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// This is the 4th bar - now indicators are ready from previous 3
	// Price drops below lower band - should get first signal
	event1 := createMREvent(decimal.NewFromInt(90))
	event1.ATR = decimal.NewFromInt(2)
	signals1 := strategy.OnMarketEvent(context.Background(), event1)
	if len(signals1) != 1 {
		t.Fatalf("Expected 1 signal on first drop, got %d", len(signals1))
	}

	// Price returns to within bands - should reset signal flag
	event2 := createMREvent(decimal.NewFromInt(100))
	event2.ATR = decimal.NewFromInt(2)
	strategy.OnMarketEvent(context.Background(), event2)

	// Now dropping below should signal again
	event3 := createMREvent(decimal.NewFromInt(85))
	event3.ATR = decimal.NewFromInt(2)
	signals := strategy.OnMarketEvent(context.Background(), event3)

	if len(signals) != 1 {
		t.Errorf("Expected signal after price returned to bands, got %d", len(signals))
	}
}

func TestMeanReversion_Bands(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	cfg.EntryStdDev = decimal.NewFromInt(2)
	strategy := NewMeanReversion(cfg)

	// Feed bars: 98, 100, 102
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	// Mean = 100
	mean := strategy.CurrentMean()
	if !mean.Equal(decimal.NewFromInt(100)) {
		t.Errorf("Mean = %s, want 100", mean)
	}

	// StdDev ≈ 1.63
	stddev := strategy.CurrentStdDev()
	expectedStdDev := decimal.RequireFromString("1.63")
	diff := stddev.Sub(expectedStdDev).Abs()
	if diff.GreaterThan(decimal.RequireFromString("0.1")) {
		t.Errorf("StdDev = %s, want approximately %s", stddev, expectedStdDev)
	}

	// Check bands
	upper, lower := strategy.Bands()

	// Upper ≈ 100 + 2*1.63 = 103.26
	// Lower ≈ 100 - 2*1.63 = 96.74
	expectedUpper := decimal.RequireFromString("103.26")
	expectedLower := decimal.RequireFromString("96.74")

	if upper.Sub(expectedUpper).Abs().GreaterThan(decimal.RequireFromString("0.2")) {
		t.Errorf("Upper band = %s, want approximately %s", upper, expectedUpper)
	}
	if lower.Sub(expectedLower).Abs().GreaterThan(decimal.RequireFromString("0.2")) {
		t.Errorf("Lower band = %s, want approximately %s", lower, expectedLower)
	}
}

func TestMeanReversion_Reset(t *testing.T) {
	cfg := DefaultMeanRevConfig()
	cfg.SMAPeriod = 3
	cfg.StdDevPeriod = 3
	strategy := NewMeanReversion(cfg)

	// Add some bars
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(98)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(100)))
	strategy.OnMarketEvent(context.Background(), createMREvent(decimal.NewFromInt(102)))

	strategy.Reset()

	// After reset, mean and stddev should be zero
	if !strategy.CurrentMean().IsZero() {
		t.Errorf("Mean after reset = %s, want 0", strategy.CurrentMean())
	}
	if !strategy.CurrentStdDev().IsZero() {
		t.Errorf("StdDev after reset = %s, want 0", strategy.CurrentStdDev())
	}
}

// Helper function
func createMREvent(close decimal.Decimal) types.MarketEvent {
	return types.MarketEvent{
		Symbol:    "MES",
		Timestamp: time.Now(),
		Open:      close,
		High:      close.Add(decimal.NewFromInt(1)),
		Low:       close.Sub(decimal.NewFromInt(1)),
		Close:     close,
	}
}
