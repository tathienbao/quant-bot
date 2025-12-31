package alerting

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestNewDailySummary(t *testing.T) {
	date := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	startEquity := decimal.NewFromInt(10000)
	endEquity := decimal.NewFromInt(10500)
	highWater := decimal.NewFromInt(11000)

	summary := NewDailySummary(
		date,
		startEquity,
		endEquity,
		highWater,
		10,  // total trades
		6,   // winning
		4,   // losing
		false,
		1,
	)

	// Check basic values
	if !summary.StartingEquity.Equal(startEquity) {
		t.Errorf("StartingEquity = %s, want %s", summary.StartingEquity, startEquity)
	}
	if !summary.EndingEquity.Equal(endEquity) {
		t.Errorf("EndingEquity = %s, want %s", summary.EndingEquity, endEquity)
	}
	if !summary.HighWaterMark.Equal(highWater) {
		t.Errorf("HighWaterMark = %s, want %s", summary.HighWaterMark, highWater)
	}

	// Check P/L
	expectedPL := decimal.NewFromInt(500)
	if !summary.TotalPL.Equal(expectedPL) {
		t.Errorf("TotalPL = %s, want %s", summary.TotalPL, expectedPL)
	}

	// Check return percentage (5%)
	expectedReturn := decimal.NewFromFloat(5)
	if !summary.ReturnPct.Equal(expectedReturn) {
		t.Errorf("ReturnPct = %s, want %s", summary.ReturnPct, expectedReturn)
	}

	// Check drawdown (~4.545%)
	// (11000 - 10500) / 11000 * 100 = 4.545...
	expectedDrawdown := decimal.NewFromFloat(4.545454545454545)
	if summary.Drawdown.Sub(expectedDrawdown).Abs().GreaterThan(decimal.NewFromFloat(0.001)) {
		t.Errorf("Drawdown = %s, want ~%s", summary.Drawdown, expectedDrawdown)
	}

	// Check win rate (60%)
	expectedWinRate := decimal.NewFromInt(60)
	if !summary.WinRate.Equal(expectedWinRate) {
		t.Errorf("WinRate = %s, want %s", summary.WinRate, expectedWinRate)
	}

	// Check counts
	if summary.TotalTrades != 10 {
		t.Errorf("TotalTrades = %d, want 10", summary.TotalTrades)
	}
	if summary.WinningTrades != 6 {
		t.Errorf("WinningTrades = %d, want 6", summary.WinningTrades)
	}
	if summary.LosingTrades != 4 {
		t.Errorf("LosingTrades = %d, want 4", summary.LosingTrades)
	}
}

func TestNewDailySummary_ZeroTrades(t *testing.T) {
	date := time.Now()
	equity := decimal.NewFromInt(10000)

	summary := NewDailySummary(
		date,
		equity,
		equity,
		equity,
		0, 0, 0,
		false,
		0,
	)

	// Win rate should be zero
	if !summary.WinRate.IsZero() {
		t.Errorf("WinRate = %s, want 0", summary.WinRate)
	}

	// Drawdown should be zero
	if !summary.Drawdown.IsZero() {
		t.Errorf("Drawdown = %s, want 0", summary.Drawdown)
	}
}

func TestNewDailySummary_NegativeReturn(t *testing.T) {
	date := time.Now()
	startEquity := decimal.NewFromInt(10000)
	endEquity := decimal.NewFromInt(9500)
	highWater := decimal.NewFromInt(10000)

	summary := NewDailySummary(
		date,
		startEquity,
		endEquity,
		highWater,
		5, 2, 3,
		true, // safe mode active
		0,
	)

	// P/L should be negative
	expectedPL := decimal.NewFromInt(-500)
	if !summary.TotalPL.Equal(expectedPL) {
		t.Errorf("TotalPL = %s, want %s", summary.TotalPL, expectedPL)
	}

	// Return should be negative (-5%)
	expectedReturn := decimal.NewFromInt(-5)
	if !summary.ReturnPct.Equal(expectedReturn) {
		t.Errorf("ReturnPct = %s, want %s", summary.ReturnPct, expectedReturn)
	}

	// Safe mode should be active
	if !summary.SafeModeActive {
		t.Error("SafeModeActive should be true")
	}
}
