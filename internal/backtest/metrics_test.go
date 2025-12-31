package backtest

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestMetrics_WinRate(t *testing.T) {
	trades := []types.Trade{
		{NetPL: decimal.NewFromInt(100)},  // Win
		{NetPL: decimal.NewFromInt(-50)},  // Loss
		{NetPL: decimal.NewFromInt(75)},   // Win
		{NetPL: decimal.NewFromInt(-25)},  // Loss
		{NetPL: decimal.NewFromInt(50)},   // Win
	}

	result := &Result{Trades: trades}
	metrics := NewMetrics(result, decimal.Zero)

	winRate := metrics.WinRate()
	expected := decimal.RequireFromString("0.6") // 3 wins out of 5

	if !winRate.Equal(expected) {
		t.Errorf("WinRate = %s, want %s", winRate, expected)
	}
}

func TestMetrics_ProfitFactor(t *testing.T) {
	trades := []types.Trade{
		{NetPL: decimal.NewFromInt(100)},  // Win
		{NetPL: decimal.NewFromInt(-50)},  // Loss
		{NetPL: decimal.NewFromInt(100)},  // Win
		{NetPL: decimal.NewFromInt(-50)},  // Loss
	}

	result := &Result{Trades: trades}
	metrics := NewMetrics(result, decimal.Zero)

	pf := metrics.ProfitFactor()
	expected := decimal.NewFromInt(2) // 200 profit / 100 loss

	if !pf.Equal(expected) {
		t.Errorf("ProfitFactor = %s, want %s", pf, expected)
	}
}

func TestMetrics_AverageWinLoss(t *testing.T) {
	trades := []types.Trade{
		{NetPL: decimal.NewFromInt(100)},
		{NetPL: decimal.NewFromInt(-50)},
		{NetPL: decimal.NewFromInt(200)},
		{NetPL: decimal.NewFromInt(-100)},
	}

	result := &Result{Trades: trades}
	metrics := NewMetrics(result, decimal.Zero)

	avgWin := metrics.AverageWin()
	expectedWin := decimal.NewFromInt(150) // (100 + 200) / 2
	if !avgWin.Equal(expectedWin) {
		t.Errorf("AverageWin = %s, want %s", avgWin, expectedWin)
	}

	avgLoss := metrics.AverageLoss()
	expectedLoss := decimal.NewFromInt(-75) // (-50 + -100) / 2
	if !avgLoss.Equal(expectedLoss) {
		t.Errorf("AverageLoss = %s, want %s", avgLoss, expectedLoss)
	}
}

func TestMetrics_MaxDrawdown(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	equityCurve := []EquityPoint{
		{Timestamp: baseTime, Equity: decimal.NewFromInt(10000)},
		{Timestamp: baseTime.Add(time.Hour), Equity: decimal.NewFromInt(11000)},  // New high
		{Timestamp: baseTime.Add(2 * time.Hour), Equity: decimal.NewFromInt(9900)},  // 10% DD
		{Timestamp: baseTime.Add(3 * time.Hour), Equity: decimal.NewFromInt(10500)}, // Partial recovery
		{Timestamp: baseTime.Add(4 * time.Hour), Equity: decimal.NewFromInt(12000)}, // New high
		{Timestamp: baseTime.Add(5 * time.Hour), Equity: decimal.NewFromInt(10800)}, // 10% DD
	}

	result := &Result{EquityCurve: equityCurve}
	metrics := NewMetrics(result, decimal.Zero)

	maxDD := metrics.MaxDrawdown()
	expected := decimal.RequireFromString("0.1") // 10% max drawdown

	if !maxDD.Equal(expected) {
		t.Errorf("MaxDrawdown = %s, want %s", maxDD, expected)
	}
}

func TestMetrics_Expectancy(t *testing.T) {
	// 50% win rate, avg win 200, avg loss -100
	// Expectancy = 0.5 * 200 + 0.5 * (-100) = 50
	trades := []types.Trade{
		{NetPL: decimal.NewFromInt(200)},
		{NetPL: decimal.NewFromInt(-100)},
	}

	result := &Result{Trades: trades}
	metrics := NewMetrics(result, decimal.Zero)

	expectancy := metrics.Expectancy()
	expected := decimal.NewFromInt(50)

	if !expectancy.Equal(expected) {
		t.Errorf("Expectancy = %s, want %s", expectancy, expected)
	}
}

func TestMetrics_SharpeRatio(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create equity curve with varying returns to have non-zero volatility
	equityValues := []int64{
		10000, 10100, 10050, 10200, 10150,
		10300, 10250, 10400, 10350, 10500,
		10450, 10600, 10550, 10700, 10650,
	}

	equityCurve := make([]EquityPoint, len(equityValues))
	for i, val := range equityValues {
		equityCurve[i] = EquityPoint{
			Timestamp: baseTime.Add(time.Duration(i) * 24 * time.Hour),
			Equity:    decimal.NewFromInt(val),
		}
	}

	result := &Result{EquityCurve: equityCurve}
	metrics := NewMetrics(result, decimal.Zero)

	sharpe := metrics.SharpeRatio()

	// With positive average returns and some volatility, Sharpe should be positive
	if sharpe.LessThanOrEqual(decimal.Zero) {
		t.Errorf("SharpeRatio should be positive for trending gains, got %s", sharpe)
	}
}

func TestMetrics_NoTrades(t *testing.T) {
	result := &Result{Trades: []types.Trade{}}
	metrics := NewMetrics(result, decimal.Zero)

	if !metrics.WinRate().IsZero() {
		t.Error("WinRate should be 0 for no trades")
	}

	if !metrics.ProfitFactor().IsZero() {
		t.Error("ProfitFactor should be 0 for no trades")
	}

	if !metrics.AverageWin().IsZero() {
		t.Error("AverageWin should be 0 for no trades")
	}

	if !metrics.AverageLoss().IsZero() {
		t.Error("AverageLoss should be 0 for no trades")
	}

	if !metrics.Expectancy().IsZero() {
		t.Error("Expectancy should be 0 for no trades")
	}
}

func TestMetrics_EmptyEquityCurve(t *testing.T) {
	result := &Result{EquityCurve: []EquityPoint{}}
	metrics := NewMetrics(result, decimal.Zero)

	if !metrics.MaxDrawdown().IsZero() {
		t.Error("MaxDrawdown should be 0 for empty curve")
	}

	if !metrics.SharpeRatio().IsZero() {
		t.Error("SharpeRatio should be 0 for empty curve")
	}

	if !metrics.SortinoRatio().IsZero() {
		t.Error("SortinoRatio should be 0 for empty curve")
	}
}

func TestMetrics_OnlyWinningTrades(t *testing.T) {
	trades := []types.Trade{
		{NetPL: decimal.NewFromInt(100)},
		{NetPL: decimal.NewFromInt(200)},
	}

	result := &Result{Trades: trades}
	metrics := NewMetrics(result, decimal.Zero)

	winRate := metrics.WinRate()
	if !winRate.Equal(decimal.NewFromInt(1)) {
		t.Errorf("WinRate = %s, want 1", winRate)
	}

	// Profit factor with no losses should return 0 (avoid division by zero)
	pf := metrics.ProfitFactor()
	if !pf.IsZero() {
		t.Errorf("ProfitFactor should be 0 when no losses, got %s", pf)
	}

	avgLoss := metrics.AverageLoss()
	if !avgLoss.IsZero() {
		t.Errorf("AverageLoss should be 0 when no losses, got %s", avgLoss)
	}
}
