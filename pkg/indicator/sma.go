// Package indicator provides technical indicator calculations.
package indicator

import (
	"github.com/shopspring/decimal"
)

// SMA calculates Simple Moving Average.
type SMA struct {
	period int
	values []decimal.Decimal
	sum    decimal.Decimal
}

// NewSMA creates a new SMA calculator with the given period.
func NewSMA(period int) *SMA {
	if period < 1 {
		period = 1
	}
	return &SMA{
		period: period,
		values: make([]decimal.Decimal, 0, period),
		sum:    decimal.Zero,
	}
}

// Update adds a new value and returns the current SMA.
// Returns zero if not enough data points yet.
func (s *SMA) Update(value decimal.Decimal) decimal.Decimal {
	s.values = append(s.values, value)
	s.sum = s.sum.Add(value)

	if len(s.values) > s.period {
		// Remove oldest value
		s.sum = s.sum.Sub(s.values[0])
		s.values = s.values[1:]
	}

	if len(s.values) < s.period {
		return decimal.Zero
	}

	return s.sum.Div(decimal.NewFromInt(int64(s.period)))
}

// Current returns the current SMA value without adding new data.
func (s *SMA) Current() decimal.Decimal {
	if len(s.values) < s.period {
		return decimal.Zero
	}
	return s.sum.Div(decimal.NewFromInt(int64(s.period)))
}

// Ready returns true if enough data points have been collected.
func (s *SMA) Ready() bool {
	return len(s.values) >= s.period
}

// Period returns the SMA period.
func (s *SMA) Period() int {
	return s.period
}

// Reset clears all data.
func (s *SMA) Reset() {
	s.values = s.values[:0]
	s.sum = decimal.Zero
}

// Count returns the number of values currently stored.
func (s *SMA) Count() int {
	return len(s.values)
}
