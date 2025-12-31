package indicator

import (
	"github.com/shopspring/decimal"
)

// StdDev calculates Standard Deviation.
type StdDev struct {
	period int
	values []decimal.Decimal
	sma    *SMA
}

// NewStdDev creates a new StdDev calculator with the given period.
func NewStdDev(period int) *StdDev {
	if period < 1 {
		period = 1
	}
	return &StdDev{
		period: period,
		values: make([]decimal.Decimal, 0, period),
		sma:    NewSMA(period),
	}
}

// Update adds a new value and returns the current standard deviation.
// Returns zero if not enough data points yet.
func (s *StdDev) Update(value decimal.Decimal) decimal.Decimal {
	s.values = append(s.values, value)
	mean := s.sma.Update(value)

	if len(s.values) > s.period {
		s.values = s.values[1:]
	}

	if len(s.values) < s.period {
		return decimal.Zero
	}

	return s.calculateStdDev(mean)
}

// Current returns the current StdDev value without adding new data.
func (s *StdDev) Current() decimal.Decimal {
	if len(s.values) < s.period {
		return decimal.Zero
	}
	mean := s.sma.Current()
	return s.calculateStdDev(mean)
}

// calculateStdDev calculates the standard deviation given the mean.
func (s *StdDev) calculateStdDev(mean decimal.Decimal) decimal.Decimal {
	if len(s.values) == 0 {
		return decimal.Zero
	}

	// Calculate variance: sum((x - mean)^2) / n
	var sumSquares decimal.Decimal
	for _, v := range s.values {
		diff := v.Sub(mean)
		sumSquares = sumSquares.Add(diff.Mul(diff))
	}

	variance := sumSquares.Div(decimal.NewFromInt(int64(len(s.values))))

	// StdDev = sqrt(variance)
	return sqrt(variance)
}

// Ready returns true if enough data points have been collected.
func (s *StdDev) Ready() bool {
	return len(s.values) >= s.period
}

// Period returns the StdDev period.
func (s *StdDev) Period() int {
	return s.period
}

// Reset clears all data.
func (s *StdDev) Reset() {
	s.values = s.values[:0]
	s.sma.Reset()
}

// Mean returns the current mean (SMA).
func (s *StdDev) Mean() decimal.Decimal {
	return s.sma.Current()
}

// sqrt calculates the square root of a decimal using Newton's method.
func sqrt(d decimal.Decimal) decimal.Decimal {
	if d.IsZero() || d.IsNegative() {
		return decimal.Zero
	}

	// Initial guess
	guess := d.Div(decimal.NewFromInt(2))
	if guess.IsZero() {
		guess = decimal.NewFromInt(1)
	}

	// Newton's method: x_new = (x + d/x) / 2
	two := decimal.NewFromInt(2)
	epsilon := decimal.RequireFromString("0.00000001")

	for i := 0; i < 100; i++ { // Max iterations
		newGuess := guess.Add(d.Div(guess)).Div(two)
		diff := newGuess.Sub(guess).Abs()
		if diff.LessThan(epsilon) {
			return newGuess.Round(8)
		}
		guess = newGuess
	}

	return guess.Round(8)
}
