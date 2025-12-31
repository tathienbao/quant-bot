package indicator

import (
	"github.com/shopspring/decimal"
)

// ATR calculates Average True Range.
// True Range = max(high - low, |high - prevClose|, |low - prevClose|)
type ATR struct {
	period    int
	prevClose decimal.Decimal
	trValues  []decimal.Decimal
	sum       decimal.Decimal
	count     int
}

// NewATR creates a new ATR calculator with the given period.
func NewATR(period int) *ATR {
	if period < 1 {
		period = 1
	}
	return &ATR{
		period:   period,
		trValues: make([]decimal.Decimal, 0, period),
	}
}

// Update calculates the True Range for the current bar and updates ATR.
// Returns the current ATR value.
func (a *ATR) Update(high, low, close decimal.Decimal) decimal.Decimal {
	var tr decimal.Decimal

	if a.count == 0 {
		// First bar: TR = high - low
		tr = high.Sub(low)
	} else {
		// TR = max(high - low, |high - prevClose|, |low - prevClose|)
		hl := high.Sub(low)
		hpc := high.Sub(a.prevClose).Abs()
		lpc := low.Sub(a.prevClose).Abs()

		tr = maxDecimal(hl, maxDecimal(hpc, lpc))
	}

	a.prevClose = close
	a.count++

	// Add to rolling window
	a.trValues = append(a.trValues, tr)
	a.sum = a.sum.Add(tr)

	if len(a.trValues) > a.period {
		a.sum = a.sum.Sub(a.trValues[0])
		a.trValues = a.trValues[1:]
	}

	if len(a.trValues) < a.period {
		return decimal.Zero
	}

	return a.sum.Div(decimal.NewFromInt(int64(a.period)))
}

// Current returns the current ATR value without adding new data.
func (a *ATR) Current() decimal.Decimal {
	if len(a.trValues) < a.period {
		return decimal.Zero
	}
	return a.sum.Div(decimal.NewFromInt(int64(a.period)))
}

// Ready returns true if enough data points have been collected.
func (a *ATR) Ready() bool {
	return len(a.trValues) >= a.period
}

// Period returns the ATR period.
func (a *ATR) Period() int {
	return a.period
}

// Reset clears all data.
func (a *ATR) Reset() {
	a.trValues = a.trValues[:0]
	a.sum = decimal.Zero
	a.prevClose = decimal.Zero
	a.count = 0
}

// maxDecimal returns the maximum of two decimals.
func maxDecimal(a, b decimal.Decimal) decimal.Decimal {
	if a.GreaterThan(b) {
		return a
	}
	return b
}
