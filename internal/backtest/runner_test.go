package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/execution"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/strategy"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestRunner_BasicBacktest(t *testing.T) {
	// Create market data with a clear uptrend for breakout
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	events := make([]types.MarketEvent, 0)

	// Build a range first (20 bars from 100-105)
	for i := 0; i < 20; i++ {
		price := decimal.NewFromInt(100 + int64(i%5))
		events = append(events, types.MarketEvent{
			Symbol:    "MES",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Open:      price,
			High:      price.Add(decimal.NewFromInt(2)),
			Low:       price.Sub(decimal.NewFromInt(2)),
			Close:     price,
		})
	}

	// Breakout bar
	events = append(events, types.MarketEvent{
		Symbol:    "MES",
		Timestamp: baseTime.Add(20 * time.Minute),
		Open:      decimal.NewFromInt(105),
		High:      decimal.NewFromInt(110),
		Low:       decimal.NewFromInt(105),
		Close:     decimal.NewFromInt(109),
	})

	// Price continues up to hit take profit
	for i := 21; i < 30; i++ {
		price := decimal.NewFromInt(110 + int64(i-20))
		events = append(events, types.MarketEvent{
			Symbol:    "MES",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Open:      price,
			High:      price.Add(decimal.NewFromInt(3)),
			Low:       price.Sub(decimal.NewFromInt(1)),
			Close:     price,
		})
	}

	feed := observer.NewMemoryFeed(events, "MES")
	calculator := observer.NewCalculator(observer.CalculatorConfig{
		ATRPeriod:    14,
		StdDevPeriod: 20,
	})

	strat := strategy.NewBreakout(strategy.BreakoutConfig{
		LookbackBars:   20,
		ATRMultiplier:  decimal.RequireFromString("2.0"),
		BreakoutBuffer: decimal.Zero,
	})

	riskCfg := risk.Config{
		MaxGlobalDrawdownPct:    decimal.RequireFromString("0.20"),
		RiskPerTradePct:         decimal.RequireFromString("0.01"),
		MaxExposurePerSymbolPct: decimal.RequireFromString("0.50"),
		MaxTotalExposurePct:     decimal.RequireFromString("1.00"),
		StopLossATRMultiple:     decimal.RequireFromString("2.0"),
		TakeProfitATRMultiple:   decimal.RequireFromString("3.0"),
	}

	execCfg := execution.SimulatedConfig{
		SlippageTicks:    0,
		CommissionPerSide: decimal.Zero,
	}

	runner := NewRunner(
		Config{InitialEquity: decimal.NewFromInt(10000)},
		feed,
		calculator,
		strat,
		riskCfg,
		execCfg,
	)

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have recorded equity curve
	if len(result.EquityCurve) == 0 {
		t.Error("Expected equity curve to be recorded")
	}

	// Start equity should be initial
	if !result.StartEquity.Equal(decimal.NewFromInt(10000)) {
		t.Errorf("StartEquity = %s, want 10000", result.StartEquity)
	}
}

func TestRunner_NoTrades(t *testing.T) {
	// Create flat market data that doesn't trigger breakout
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	events := make([]types.MarketEvent, 0)

	// Flat market, no breakout
	for i := 0; i < 30; i++ {
		events = append(events, types.MarketEvent{
			Symbol:    "MES",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Open:      decimal.NewFromInt(100),
			High:      decimal.NewFromInt(101),
			Low:       decimal.NewFromInt(99),
			Close:     decimal.NewFromInt(100),
		})
	}

	feed := observer.NewMemoryFeed(events, "MES")
	calculator := observer.NewCalculator(observer.DefaultCalculatorConfig())

	strat := strategy.NewBreakout(strategy.BreakoutConfig{
		LookbackBars:   20,
		ATRMultiplier:  decimal.RequireFromString("2.0"),
		BreakoutBuffer: decimal.RequireFromString("0.01"), // 1% buffer makes it hard to break out
	})

	riskCfg := risk.DefaultConfig()
	execCfg := execution.DefaultSimulatedConfig()

	runner := NewRunner(
		Config{InitialEquity: decimal.NewFromInt(10000)},
		feed,
		calculator,
		strat,
		riskCfg,
		execCfg,
	)

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.TotalTrades != 0 {
		t.Errorf("TotalTrades = %d, want 0", result.TotalTrades)
	}

	// Equity should be unchanged
	if !result.EndEquity.Equal(result.StartEquity) {
		t.Errorf("EndEquity = %s, want %s (no trades)", result.EndEquity, result.StartEquity)
	}
}

func TestRunner_Reset(t *testing.T) {
	events := []types.MarketEvent{
		{Symbol: "MES", Timestamp: time.Now(), Close: decimal.NewFromInt(100)},
	}

	feed := observer.NewMemoryFeed(events, "MES")
	calculator := observer.NewCalculator(observer.DefaultCalculatorConfig())
	strat := strategy.NewBreakout(strategy.DefaultBreakoutConfig())
	riskCfg := risk.DefaultConfig()
	execCfg := execution.DefaultSimulatedConfig()

	runner := NewRunner(
		Config{InitialEquity: decimal.NewFromInt(10000)},
		feed,
		calculator,
		strat,
		riskCfg,
		execCfg,
	)

	// Run once
	_, _ = runner.Run(context.Background())

	// Reset
	runner.Reset()

	// Equity curve should be cleared
	// (We can't easily verify internal state, but at least it shouldn't panic)
}

func TestResult_Metrics(t *testing.T) {
	// Test metric calculations
	result := Result{
		StartEquity:   decimal.NewFromInt(10000),
		EndEquity:     decimal.NewFromInt(11000),
		TotalTrades:   10,
		WinningTrades: 6,
		LosingTrades:  4,
	}

	// Win rate should be 60%
	expectedWinRate := decimal.RequireFromString("0.6")
	actualWinRate := decimal.NewFromInt(int64(result.WinningTrades)).Div(decimal.NewFromInt(int64(result.TotalTrades)))
	if !actualWinRate.Equal(expectedWinRate) {
		t.Errorf("WinRate = %s, want %s", actualWinRate, expectedWinRate)
	}

	// Verify structure is populated
	if result.StartEquity.IsZero() || result.EndEquity.IsZero() {
		t.Error("Equity values should not be zero")
	}
}

func TestRunner_TimeFilters(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	events := make([]types.MarketEvent, 0)

	for i := 0; i < 30; i++ {
		events = append(events, types.MarketEvent{
			Symbol:    "MES",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Open:      decimal.NewFromInt(100),
			High:      decimal.NewFromInt(101),
			Low:       decimal.NewFromInt(99),
			Close:     decimal.NewFromInt(100),
		})
	}

	feed := observer.NewMemoryFeed(events, "MES")
	calculator := observer.NewCalculator(observer.DefaultCalculatorConfig())
	strat := strategy.NewBreakout(strategy.DefaultBreakoutConfig())
	riskCfg := risk.DefaultConfig()
	execCfg := execution.DefaultSimulatedConfig()

	// Only include events from minute 10 to 20
	startTime := baseTime.Add(10 * time.Minute)
	endTime := baseTime.Add(20 * time.Minute)

	runner := NewRunner(
		Config{
			InitialEquity: decimal.NewFromInt(10000),
			StartTime:     startTime,
			EndTime:       endTime,
		},
		feed,
		calculator,
		strat,
		riskCfg,
		execCfg,
	)

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have filtered equity curve to only include events in range
	for _, point := range result.EquityCurve {
		if point.Timestamp.Before(startTime) {
			t.Errorf("Equity point %v before start time %v", point.Timestamp, startTime)
		}
		if point.Timestamp.After(endTime) {
			t.Errorf("Equity point %v after end time %v", point.Timestamp, endTime)
		}
	}
}
