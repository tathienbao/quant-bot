package risk

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func TestPositionSizer_Calculate(t *testing.T) {
	// MES tick value = $1.25
	sizer := NewPositionSizer(decimal.RequireFromString("1.25"))

	tests := []struct {
		name          string
		equity        string
		riskPct       string
		stopTicks     int
		wantContracts int
	}{
		{
			name:          "insufficient equity for 1 contract",
			equity:        "1000",
			riskPct:       "0.01", // $10 risk
			stopTicks:     10,     // $12.50 per contract risk
			wantContracts: 0,      // 10 / 12.5 = 0.8 < 1
		},
		{
			name:          "exactly 1 contract",
			equity:        "1250",
			riskPct:       "0.01", // $12.50 risk
			stopTicks:     10,     // $12.50 per contract risk
			wantContracts: 1,
		},
		{
			name:          "multiple contracts",
			equity:        "10000",
			riskPct:       "0.01", // $100 risk
			stopTicks:     10,     // $12.50 per contract risk
			wantContracts: 8,      // 100 / 12.5 = 8
		},
		{
			name:          "high volatility reduces size",
			equity:        "10000",
			riskPct:       "0.01", // $100 risk
			stopTicks:     50,     // $62.50 per contract risk
			wantContracts: 1,      // 100 / 62.5 = 1.6 -> 1
		},
		{
			name:          "low volatility increases size",
			equity:        "10000",
			riskPct:       "0.01", // $100 risk
			stopTicks:     5,      // $6.25 per contract risk
			wantContracts: 16,     // 100 / 6.25 = 16
		},
		{
			name:          "higher risk percentage",
			equity:        "10000",
			riskPct:       "0.02", // $200 risk
			stopTicks:     10,     // $12.50 per contract risk
			wantContracts: 16,     // 200 / 12.5 = 16
		},
		{
			name:          "larger account",
			equity:        "50000",
			riskPct:       "0.01", // $500 risk
			stopTicks:     10,     // $12.50 per contract risk
			wantContracts: 40,     // 500 / 12.5 = 40
		},
		{
			name:          "zero stop ticks",
			equity:        "10000",
			riskPct:       "0.01",
			stopTicks:     0,
			wantContracts: 0,
		},
		{
			name:          "negative stop ticks",
			equity:        "10000",
			riskPct:       "0.01",
			stopTicks:     -5,
			wantContracts: 0,
		},
		{
			name:          "zero equity",
			equity:        "0",
			riskPct:       "0.01",
			stopTicks:     10,
			wantContracts: 0,
		},
		{
			name:          "zero risk percentage",
			equity:        "10000",
			riskPct:       "0",
			stopTicks:     10,
			wantContracts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sizer.Calculate(
				decimal.RequireFromString(tt.equity),
				decimal.RequireFromString(tt.riskPct),
				tt.stopTicks,
			)
			if got != tt.wantContracts {
				t.Errorf("Calculate() = %d, want %d", got, tt.wantContracts)
			}
		})
	}
}

func TestPositionSizer_CalculateWithDetails(t *testing.T) {
	// MES: tick size = 0.25, tick value = $1.25
	sizer := NewPositionSizer(decimal.RequireFromString("1.25"))
	tickSize := decimal.RequireFromString("0.25")

	tests := []struct {
		name          string
		equity        string
		riskPct       string
		stopTicks     int
		entryPrice    string
		side          types.Side
		wantValid     bool
		wantContracts int
		wantStopLoss  string
	}{
		{
			name:          "valid long position",
			equity:        "10000",
			riskPct:       "0.01",
			stopTicks:     10,
			entryPrice:    "5000.00",
			side:          types.SideLong,
			wantValid:     true,
			wantContracts: 8,
			wantStopLoss:  "4997.50", // 5000 - (10 * 0.25)
		},
		{
			name:          "valid short position",
			equity:        "10000",
			riskPct:       "0.01",
			stopTicks:     10,
			entryPrice:    "5000.00",
			side:          types.SideShort,
			wantValid:     true,
			wantContracts: 8,
			wantStopLoss:  "5002.50", // 5000 + (10 * 0.25)
		},
		{
			name:          "insufficient equity",
			equity:        "500",
			riskPct:       "0.01",
			stopTicks:     10,
			entryPrice:    "5000.00",
			side:          types.SideLong,
			wantValid:     false,
			wantContracts: 0,
		},
		{
			name:          "risk too high (over 10%)",
			equity:        "10000",
			riskPct:       "0.15",
			stopTicks:     10,
			entryPrice:    "5000.00",
			side:          types.SideLong,
			wantValid:     false,
			wantContracts: 0,
		},
		{
			name:          "zero stop distance",
			equity:        "10000",
			riskPct:       "0.01",
			stopTicks:     0,
			entryPrice:    "5000.00",
			side:          types.SideLong,
			wantValid:     false,
			wantContracts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sizer.CalculateWithDetails(
				decimal.RequireFromString(tt.equity),
				decimal.RequireFromString(tt.riskPct),
				tt.stopTicks,
				decimal.RequireFromString(tt.entryPrice),
				tt.side,
				tickSize,
			)

			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v (reason: %s)", result.Valid, tt.wantValid, result.RejectReason)
			}

			if result.Contracts != tt.wantContracts {
				t.Errorf("Contracts = %d, want %d", result.Contracts, tt.wantContracts)
			}

			if tt.wantValid && tt.wantStopLoss != "" {
				wantSL := decimal.RequireFromString(tt.wantStopLoss)
				if !result.StopLoss.Equal(wantSL) {
					t.Errorf("StopLoss = %s, want %s", result.StopLoss, wantSL)
				}
			}
		})
	}
}

func TestPositionSizer_RiskAmountCalculation(t *testing.T) {
	sizer := NewPositionSizer(decimal.RequireFromString("1.25"))
	tickSize := decimal.RequireFromString("0.25")

	result := sizer.CalculateWithDetails(
		decimal.RequireFromString("10000"), // equity
		decimal.RequireFromString("0.01"),  // 1% risk = $100
		10,                                 // stop ticks
		decimal.RequireFromString("5000"),
		types.SideLong,
		tickSize,
	)

	if !result.Valid {
		t.Fatalf("Expected valid result, got: %s", result.RejectReason)
	}

	// 8 contracts * 10 ticks * $1.25/tick = $100
	expectedRisk := decimal.RequireFromString("100")
	if !result.RiskAmount.Equal(expectedRisk) {
		t.Errorf("RiskAmount = %s, want %s", result.RiskAmount, expectedRisk)
	}
}

func TestPositionSizer_MaxContracts(t *testing.T) {
	sizer := NewPositionSizer(decimal.RequireFromString("1.25"))

	tests := []struct {
		name          string
		equity        string
		maxExposure   string
		price         string
		pointValue    string
		wantContracts int
	}{
		{
			name:          "50% exposure limit",
			equity:        "10000",
			maxExposure:   "0.5",    // 50% = $5000
			price:         "5000",   // ES price
			pointValue:    "5",      // MES $5/point
			wantContracts: 0,        // 5000 / (5000*5) = 0.2
		},
		{
			name:          "high equity allows more contracts",
			equity:        "100000",
			maxExposure:   "0.5",    // 50% = $50000
			price:         "5000",
			pointValue:    "5",
			wantContracts: 2, // 50000 / 25000 = 2
		},
		{
			name:          "zero price",
			equity:        "10000",
			maxExposure:   "0.5",
			price:         "0",
			pointValue:    "5",
			wantContracts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sizer.MaxContracts(
				decimal.RequireFromString(tt.equity),
				decimal.RequireFromString(tt.maxExposure),
				decimal.RequireFromString(tt.price),
				decimal.RequireFromString(tt.pointValue),
			)
			if got != tt.wantContracts {
				t.Errorf("MaxContracts() = %d, want %d", got, tt.wantContracts)
			}
		})
	}
}

func TestPositionSizer_AdjustForMaxSize(t *testing.T) {
	sizer := NewPositionSizer(decimal.RequireFromString("1.25"))

	tests := []struct {
		calculated int
		maxAllowed int
		want       int
	}{
		{10, 5, 5},   // Exceeds max
		{3, 5, 3},    // Under max
		{5, 5, 5},    // Equal to max
		{0, 5, 0},    // Zero
		{10, 0, 0},   // Zero max
	}

	for _, tt := range tests {
		got := sizer.AdjustForMaxSize(tt.calculated, tt.maxAllowed)
		if got != tt.want {
			t.Errorf("AdjustForMaxSize(%d, %d) = %d, want %d",
				tt.calculated, tt.maxAllowed, got, tt.want)
		}
	}
}

func TestNewPositionSizerForSymbol(t *testing.T) {
	tests := []struct {
		symbol    string
		wantErr   bool
		wantTick  string
	}{
		{"MES", false, "1.25"},
		{"MGC", false, "1.00"},
		{"UNKNOWN", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			sizer, err := NewPositionSizerForSymbol(tt.symbol)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for symbol %s", tt.symbol)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify tick value by calculating a known case
			// With $10000 equity, 1% risk, 8 tick stop:
			// MES: 100 / (8 * 1.25) = 10 contracts
			// MGC: 100 / (8 * 1.00) = 12 contracts
			contracts := sizer.Calculate(
				decimal.RequireFromString("10000"),
				decimal.RequireFromString("0.01"),
				8,
			)

			expectedContracts := map[string]int{
				"MES": 10, // 100 / 10 = 10
				"MGC": 12, // 100 / 8 = 12
			}

			if contracts != expectedContracts[tt.symbol] {
				t.Errorf("For %s, got %d contracts, want %d",
					tt.symbol, contracts, expectedContracts[tt.symbol])
			}
		})
	}
}
