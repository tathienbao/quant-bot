package types

import (
	"testing"

	"github.com/shopspring/decimal"
)

// TestSide_String tests Side string conversion.
func TestSide_String(t *testing.T) {
	tests := []struct {
		side Side
		want string
	}{
		{SideLong, "LONG"},
		{SideShort, "SHORT"},
		{SideFlat, "FLAT"},
		{Side(99), "FLAT"}, // Unknown defaults to FLAT
	}

	for _, tt := range tests {
		got := tt.side.String()
		if got != tt.want {
			t.Errorf("Side(%d).String() = %s, want %s", tt.side, got, tt.want)
		}
	}
}

// TestSide_Opposite tests direction flip.
func TestSide_Opposite(t *testing.T) {
	tests := []struct {
		side Side
		want Side
	}{
		{SideLong, SideShort},
		{SideShort, SideLong},
		{SideFlat, SideFlat},
	}

	for _, tt := range tests {
		got := tt.side.Opposite()
		if got != tt.want {
			t.Errorf("Side(%d).Opposite() = %d, want %d", tt.side, got, tt.want)
		}
	}
}

// TestOrderStatus_String tests status string conversion.
func TestOrderStatus_String(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   string
	}{
		{OrderStatusCreated, "CREATED"},
		{OrderStatusPending, "PENDING"},
		{OrderStatusPartialFill, "PARTIAL_FILL"},
		{OrderStatusFilled, "FILLED"},
		{OrderStatusRejected, "REJECTED"},
		{OrderStatusCancelled, "CANCELLED"},
		{OrderStatusExpired, "EXPIRED"},
		{OrderStatus(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		got := tt.status.String()
		if got != tt.want {
			t.Errorf("OrderStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

// TestOrderStatus_IsFinal tests terminal state check.
func TestOrderStatus_IsFinal(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   bool
	}{
		{OrderStatusCreated, false},
		{OrderStatusPending, false},
		{OrderStatusPartialFill, false},
		{OrderStatusFilled, true},
		{OrderStatusRejected, true},
		{OrderStatusCancelled, true},
		{OrderStatusExpired, true},
	}

	for _, tt := range tests {
		got := tt.status.IsFinal()
		if got != tt.want {
			t.Errorf("OrderStatus(%d).IsFinal() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// TestDecimal_FloatPrecision tests 0.1 + 0.2 = 0.3 (DEC-01).
func TestDecimal_FloatPrecision(t *testing.T) {
	a := decimal.RequireFromString("0.1")
	b := decimal.RequireFromString("0.2")
	expected := decimal.RequireFromString("0.3")

	result := a.Add(b)
	if !result.Equal(expected) {
		t.Errorf("0.1 + 0.2 = %s, want 0.3 (DEC-01)", result.String())
	}
}

// TestDecimal_Accumulated tests 1000 * $0.01 = $10.00 (DEC-02).
func TestDecimal_Accumulated(t *testing.T) {
	amount := decimal.RequireFromString("0.01")
	count := 1000
	expected := decimal.RequireFromString("10.00")

	result := decimal.Zero
	for i := 0; i < count; i++ {
		result = result.Add(amount)
	}

	if !result.Equal(expected) {
		t.Errorf("1000 * $0.01 = %s, want $10.00 (DEC-02)", result.String())
	}
}

// TestDecimal_LargeValues tests large P&L values (DEC-03).
func TestDecimal_LargeValues(t *testing.T) {
	largeValue := decimal.RequireFromString("250000.00")
	multiplier := decimal.RequireFromString("1.5")
	expected := decimal.RequireFromString("375000.00")

	result := largeValue.Mul(multiplier)
	if !result.Equal(expected) {
		t.Errorf("250000 * 1.5 = %s, want 375000 (DEC-03)", result.String())
	}

	// Test no overflow
	veryLarge := decimal.RequireFromString("999999999999.99")
	if veryLarge.IsZero() {
		t.Error("large value should not be zero")
	}
}

// TestGetInstrumentSpec tests instrument spec lookup.
func TestGetInstrumentSpec(t *testing.T) {
	// MES
	mesSpec, ok := GetInstrumentSpec("MES")
	if !ok {
		t.Fatal("expected MES spec")
	}
	if mesSpec.Symbol != "MES" {
		t.Errorf("MES symbol = %s, want MES", mesSpec.Symbol)
	}
	expectedTickSize := decimal.RequireFromString("0.25")
	if !mesSpec.TickSize.Equal(expectedTickSize) {
		t.Errorf("MES tick size = %s, want 0.25", mesSpec.TickSize.String())
	}

	// MGC
	mgcSpec, ok := GetInstrumentSpec("MGC")
	if !ok {
		t.Fatal("expected MGC spec")
	}
	if mgcSpec.Symbol != "MGC" {
		t.Errorf("MGC symbol = %s, want MGC", mgcSpec.Symbol)
	}

	// Unknown
	_, ok = GetInstrumentSpec("INVALID")
	if ok {
		t.Error("expected false for unknown symbol")
	}
}

// TestInstrumentSpec_TicksToPrice tests tick conversion.
func TestInstrumentSpec_TicksToPrice(t *testing.T) {
	mesSpec, ok := GetInstrumentSpec("MES")
	if !ok {
		t.Fatal("expected MES spec")
	}

	// 10 ticks = 2.5 points for MES
	ticks := 10
	expected := decimal.RequireFromString("2.5") // 10 * 0.25
	result := mesSpec.TickSize.Mul(decimal.NewFromInt(int64(ticks)))

	if !result.Equal(expected) {
		t.Errorf("10 ticks = %s, want 2.5", result.String())
	}
}
