package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestATR_Basic(t *testing.T) {
	atr := NewATR(3)

	// Not ready yet
	if atr.Ready() {
		t.Error("ATR should not be ready with no data")
	}

	// Add bars: each has TR = high - low = 10
	// Bar 1: H=110, L=100, C=105 -> TR = 10
	atr.Update(d("110"), d("100"), d("105"))

	// Bar 2: H=115, L=105, C=110 -> TR = max(10, |115-105|=10, |105-105|=0) = 10
	atr.Update(d("115"), d("105"), d("110"))

	// Bar 3: H=120, L=110, C=115 -> TR = max(10, |120-110|=10, |110-110|=0) = 10
	result := atr.Update(d("120"), d("110"), d("115"))

	// ATR(3) = average of [10, 10, 10] = 10
	expected := d("10")
	if !result.Equal(expected) {
		t.Errorf("ATR = %s, want %s", result, expected)
	}
}

func TestATR_GapUp(t *testing.T) {
	atr := NewATR(2)

	// Bar 1: H=110, L=100, C=105 -> TR = 10
	atr.Update(d("110"), d("100"), d("105"))

	// Bar 2: Gap up - H=125, L=115, C=120
	// TR = max(10, |125-105|=20, |115-105|=10) = 20
	result := atr.Update(d("125"), d("115"), d("120"))

	// ATR(2) = average of [10, 20] = 15
	expected := d("15")
	if !result.Equal(expected) {
		t.Errorf("ATR with gap = %s, want %s", result, expected)
	}
}

func TestATR_GapDown(t *testing.T) {
	atr := NewATR(2)

	// Bar 1: H=110, L=100, C=105 -> TR = 10
	atr.Update(d("110"), d("100"), d("105"))

	// Bar 2: Gap down - H=95, L=85, C=90
	// TR = max(10, |95-105|=10, |85-105|=20) = 20
	result := atr.Update(d("95"), d("85"), d("90"))

	// ATR(2) = average of [10, 20] = 15
	expected := d("15")
	if !result.Equal(expected) {
		t.Errorf("ATR with gap = %s, want %s", result, expected)
	}
}

func TestATR_Reset(t *testing.T) {
	atr := NewATR(3)

	atr.Update(d("110"), d("100"), d("105"))
	atr.Update(d("115"), d("105"), d("110"))
	atr.Update(d("120"), d("110"), d("115"))

	atr.Reset()

	if atr.Ready() {
		t.Error("ATR should not be ready after reset")
	}

	if !atr.Current().IsZero() {
		t.Errorf("Current = %s, want 0", atr.Current())
	}
}

func TestATR_Rolling(t *testing.T) {
	atr := NewATR(2)

	// Bar 1: H=110, L=100, C=105 -> TR = 10 (first bar)
	atr.Update(d("110"), d("100"), d("105"))
	// Bar 2: H=115, L=105, C=110 -> TR = max(10, |115-105|=10, |105-105|=0) = 10
	atr.Update(d("115"), d("105"), d("110"))
	// Bar 3: H=120, L=110, C=115 -> TR = max(10, |120-110|=10, |110-110|=0) = 10
	result := atr.Update(d("120"), d("110"), d("115"))

	// ATR(2) of [10, 10] = 10 (first TR dropped, now window is [10, 10])
	expected := d("10")
	if !result.Equal(expected) {
		t.Errorf("Rolling ATR = %s, want %s", result, expected)
	}
}

// Helper to create decimals
func d(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}
