package broker

import (
	"testing"
	"time"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state ConnectionState
		want  string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
		{StateError, "error"},
		{ConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("ConnectionState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMESContract(t *testing.T) {
	contract := MESContract("202503")

	if contract.Symbol != "MES" {
		t.Errorf("Symbol = %s, want MES", contract.Symbol)
	}
	if contract.SecType != "FUT" {
		t.Errorf("SecType = %s, want FUT", contract.SecType)
	}
	if contract.Exchange != "CME" {
		t.Errorf("Exchange = %s, want CME", contract.Exchange)
	}
	if contract.Multiplier != 5 {
		t.Errorf("Multiplier = %d, want 5", contract.Multiplier)
	}
	if contract.Expiry != "202503" {
		t.Errorf("Expiry = %s, want 202503", contract.Expiry)
	}
}

func TestMGCContract(t *testing.T) {
	contract := MGCContract("202503")

	if contract.Symbol != "MGC" {
		t.Errorf("Symbol = %s, want MGC", contract.Symbol)
	}
	if contract.SecType != "FUT" {
		t.Errorf("SecType = %s, want FUT", contract.SecType)
	}
	if contract.Exchange != "COMEX" {
		t.Errorf("Exchange = %s, want COMEX", contract.Exchange)
	}
	if contract.Multiplier != 10 {
		t.Errorf("Multiplier = %d, want 10", contract.Multiplier)
	}
}

func TestGetFrontMonthExpiry(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		want string
	}{
		{
			name: "early january",
			date: time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC),
			want: "202503", // March
		},
		{
			name: "mid march before expiry",
			date: time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC),
			want: "202503", // March (before 3rd Friday)
		},
		{
			name: "after march expiry",
			date: time.Date(2025, 3, 25, 0, 0, 0, 0, time.UTC),
			want: "202506", // June
		},
		{
			name: "december before expiry",
			date: time.Date(2025, 12, 10, 0, 0, 0, 0, time.UTC),
			want: "202512", // December
		},
		{
			name: "december after expiry",
			date: time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC),
			want: "202603", // Next year March
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFrontMonthExpiry(tt.date)
			if got != tt.want {
				t.Errorf("GetFrontMonthExpiry() = %v, want %v", got, tt.want)
			}
		})
	}
}
