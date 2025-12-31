package risk

import (
	"testing"

	"github.com/shopspring/decimal"
)

// FuzzPositionSizer tests position sizing with random inputs.
func FuzzPositionSizer(f *testing.F) {
	// Add seed corpus
	f.Add("10000.00", "0.01", 10, "1.25")
	f.Add("1000.00", "0.02", 5, "1.25")
	f.Add("0.00", "0.00", 0, "1.25")
	f.Add("999999.99", "0.10", 100, "5.00")
	f.Add("50.00", "0.01", 1, "1.25")

	f.Fuzz(func(t *testing.T, equityStr string, riskPctStr string, stopTicks int, tickValueStr string) {
		// Parse inputs - skip invalid
		equity, err := decimal.NewFromString(equityStr)
		if err != nil || equity.LessThan(decimal.Zero) {
			return
		}

		riskPct, err := decimal.NewFromString(riskPctStr)
		if err != nil || riskPct.LessThan(decimal.Zero) || riskPct.GreaterThan(decimal.NewFromInt(1)) {
			return
		}

		tickValue, err := decimal.NewFromString(tickValueStr)
		if err != nil || tickValue.LessThanOrEqual(decimal.Zero) {
			return
		}

		if stopTicks < 0 || stopTicks > 10000 {
			return
		}

		// Create sizer with tickValue
		sizer := NewPositionSizer(tickValue)

		// Should never panic
		contracts := sizer.Calculate(equity, riskPct, stopTicks)

		// Invariant: contracts >= 0
		if contracts < 0 {
			t.Errorf("negative contracts: %d", contracts)
		}
	})
}

// FuzzDrawdownCalculation tests drawdown calculation with random equity values.
func FuzzDrawdownCalculation(f *testing.F) {
	// Seed corpus
	f.Add("10000.00", "10000.00")
	f.Add("12000.00", "10000.00")
	f.Add("8000.00", "10000.00")
	f.Add("0.01", "10000.00")
	f.Add("10000.00", "0.01")

	f.Fuzz(func(t *testing.T, equityStr string, peakStr string) {
		equity, err := decimal.NewFromString(equityStr)
		if err != nil || equity.LessThanOrEqual(decimal.Zero) {
			return
		}

		peak, err := decimal.NewFromString(peakStr)
		if err != nil || peak.LessThanOrEqual(decimal.Zero) {
			return
		}

		tracker := NewHighWaterMarkTracker(peak)
		tracker.Update(equity)
		current, hwm, drawdown := tracker.Snapshot()

		// Invariants
		// 1. Drawdown should be non-negative
		if drawdown.LessThan(decimal.Zero) {
			t.Errorf("negative drawdown: %s", drawdown)
		}

		// 2. Drawdown should be <= 1 (100%)
		if drawdown.GreaterThan(decimal.NewFromInt(1)) {
			t.Errorf("drawdown > 100%%: %s", drawdown)
		}

		// 3. HWM should be >= equity (when equity was higher)
		if hwm.LessThan(current) && equity.GreaterThan(peak) {
			t.Error("HWM should track new highs")
		}
	})
}

// FuzzDecimalArithmetic tests decimal operations don't lose precision.
func FuzzDecimalArithmetic(f *testing.F) {
	f.Add("100.00", "0.01", 1000)
	f.Add("1234.56", "0.99", 100)
	f.Add("0.01", "0.01", 10)

	f.Fuzz(func(t *testing.T, baseStr string, incrementStr string, count int) {
		base, err := decimal.NewFromString(baseStr)
		if err != nil || base.LessThan(decimal.Zero) {
			return
		}

		increment, err := decimal.NewFromString(incrementStr)
		if err != nil {
			return
		}

		if count < 0 || count > 10000 {
			return
		}

		// Accumulate
		result := base
		for i := 0; i < count; i++ {
			result = result.Add(increment)
		}

		// Calculate expected
		expected := base.Add(increment.Mul(decimal.NewFromInt(int64(count))))

		// Should match exactly (no floating point errors)
		if !result.Equal(expected) {
			t.Errorf("precision loss: got %s, want %s", result, expected)
		}
	})
}
