package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestStdDev_Basic(t *testing.T) {
	stddev := NewStdDev(3)

	// Not ready yet
	if stddev.Ready() {
		t.Error("StdDev should not be ready with no data")
	}

	// Add values: 10, 20, 30
	// Mean = 20
	// Variance = ((10-20)^2 + (20-20)^2 + (30-20)^2) / 3 = (100 + 0 + 100) / 3 = 66.67
	// StdDev = sqrt(66.67) ≈ 8.16
	stddev.Update(decimal.NewFromInt(10))
	stddev.Update(decimal.NewFromInt(20))
	result := stddev.Update(decimal.NewFromInt(30))

	if !stddev.Ready() {
		t.Error("StdDev should be ready after 3 values")
	}

	// Check approximately
	expected := decimal.RequireFromString("8.16")
	diff := result.Sub(expected).Abs()
	if diff.GreaterThan(decimal.RequireFromString("0.01")) {
		t.Errorf("StdDev = %s, want approximately %s", result, expected)
	}
}

func TestStdDev_ZeroVariance(t *testing.T) {
	stddev := NewStdDev(3)

	// All same values
	stddev.Update(decimal.NewFromInt(10))
	stddev.Update(decimal.NewFromInt(10))
	result := stddev.Update(decimal.NewFromInt(10))

	// StdDev should be 0
	if !result.IsZero() {
		t.Errorf("StdDev of identical values = %s, want 0", result)
	}
}

func TestStdDev_Mean(t *testing.T) {
	stddev := NewStdDev(3)

	stddev.Update(decimal.NewFromInt(10))
	stddev.Update(decimal.NewFromInt(20))
	stddev.Update(decimal.NewFromInt(30))

	mean := stddev.Mean()
	expected := decimal.NewFromInt(20)
	if !mean.Equal(expected) {
		t.Errorf("Mean = %s, want %s", mean, expected)
	}
}

func TestStdDev_Reset(t *testing.T) {
	stddev := NewStdDev(3)

	stddev.Update(decimal.NewFromInt(10))
	stddev.Update(decimal.NewFromInt(20))
	stddev.Update(decimal.NewFromInt(30))

	stddev.Reset()

	if stddev.Ready() {
		t.Error("StdDev should not be ready after reset")
	}
}

func TestStdDev_Rolling(t *testing.T) {
	stddev := NewStdDev(3)

	// Add 4 values, window should only contain last 3
	stddev.Update(decimal.NewFromInt(100)) // Will be dropped
	stddev.Update(decimal.NewFromInt(10))
	stddev.Update(decimal.NewFromInt(20))
	result := stddev.Update(decimal.NewFromInt(30))

	// Mean of [10, 20, 30] = 20
	// StdDev ≈ 8.16
	expected := decimal.RequireFromString("8.16")
	diff := result.Sub(expected).Abs()
	if diff.GreaterThan(decimal.RequireFromString("0.01")) {
		t.Errorf("Rolling StdDev = %s, want approximately %s", result, expected)
	}
}

func TestSqrt(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0", "0"},
		{"1", "1"},
		{"4", "2"},
		{"9", "3"},
		{"2", "1.41421356"},
		{"100", "10"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			input := decimal.RequireFromString(tt.input)
			expected := decimal.RequireFromString(tt.expected)
			result := sqrt(input)

			diff := result.Sub(expected).Abs()
			if diff.GreaterThan(decimal.RequireFromString("0.0001")) {
				t.Errorf("sqrt(%s) = %s, want %s", tt.input, result, expected)
			}
		})
	}
}
