package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestSMA_Basic(t *testing.T) {
	sma := NewSMA(3)

	// Not ready yet
	if sma.Ready() {
		t.Error("SMA should not be ready with no data")
	}

	// Add values: 10, 20, 30
	sma.Update(decimal.NewFromInt(10))
	sma.Update(decimal.NewFromInt(20))
	result := sma.Update(decimal.NewFromInt(30))

	// SMA(3) of [10, 20, 30] = 20
	expected := decimal.NewFromInt(20)
	if !result.Equal(expected) {
		t.Errorf("SMA = %s, want %s", result, expected)
	}

	if !sma.Ready() {
		t.Error("SMA should be ready after 3 values")
	}
}

func TestSMA_Rolling(t *testing.T) {
	sma := NewSMA(3)

	// Add values: 10, 20, 30, 40
	sma.Update(decimal.NewFromInt(10))
	sma.Update(decimal.NewFromInt(20))
	sma.Update(decimal.NewFromInt(30))
	result := sma.Update(decimal.NewFromInt(40))

	// SMA(3) of [20, 30, 40] = 30
	expected := decimal.NewFromInt(30)
	if !result.Equal(expected) {
		t.Errorf("SMA = %s, want %s", result, expected)
	}
}

func TestSMA_NotReady(t *testing.T) {
	sma := NewSMA(5)

	// Add only 3 values
	sma.Update(decimal.NewFromInt(10))
	sma.Update(decimal.NewFromInt(20))
	result := sma.Update(decimal.NewFromInt(30))

	// Should return zero when not ready
	if !result.IsZero() {
		t.Errorf("SMA should be zero when not ready, got %s", result)
	}
}

func TestSMA_Reset(t *testing.T) {
	sma := NewSMA(3)

	sma.Update(decimal.NewFromInt(10))
	sma.Update(decimal.NewFromInt(20))
	sma.Update(decimal.NewFromInt(30))

	sma.Reset()

	if sma.Ready() {
		t.Error("SMA should not be ready after reset")
	}

	if sma.Count() != 0 {
		t.Errorf("Count = %d, want 0", sma.Count())
	}
}

func TestSMA_Current(t *testing.T) {
	sma := NewSMA(3)

	sma.Update(decimal.NewFromInt(10))
	sma.Update(decimal.NewFromInt(20))
	sma.Update(decimal.NewFromInt(30))

	// Current should return the same as last Update
	current := sma.Current()
	expected := decimal.NewFromInt(20)
	if !current.Equal(expected) {
		t.Errorf("Current = %s, want %s", current, expected)
	}
}
