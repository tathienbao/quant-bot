package risk

import (
	"sync"
	"testing"

	"github.com/shopspring/decimal"
)

func TestHighWaterMarkTracker_NewTracker(t *testing.T) {
	initial := decimal.RequireFromString("1000")
	tracker := NewHighWaterMarkTracker(initial)

	if !tracker.Current().Equal(initial) {
		t.Errorf("Current() = %s, want %s", tracker.Current(), initial)
	}

	if !tracker.Peak().Equal(initial) {
		t.Errorf("Peak() = %s, want %s", tracker.Peak(), initial)
	}

	if !tracker.Drawdown().IsZero() {
		t.Errorf("Drawdown() = %s, want 0", tracker.Drawdown())
	}
}

func TestHighWaterMarkTracker_Update(t *testing.T) {
	tests := []struct {
		name         string
		initial      string
		updates      []string
		wantCurrent  string
		wantPeak     string
		wantDrawdown string
	}{
		{
			name:         "equity increases - new peak",
			initial:      "1000",
			updates:      []string{"1100"},
			wantCurrent:  "1100",
			wantPeak:     "1100",
			wantDrawdown: "0",
		},
		{
			name:         "equity decreases - drawdown",
			initial:      "1000",
			updates:      []string{"1100", "990"},
			wantCurrent:  "990",
			wantPeak:     "1100",
			wantDrawdown: "0.1", // 10%
		},
		{
			name:         "equity recovers partially",
			initial:      "1000",
			updates:      []string{"1100", "990", "1050"},
			wantCurrent:  "1050",
			wantPeak:     "1100",
			wantDrawdown: "0.0454545454545454545454545454545455", // ~4.55%
		},
		{
			name:         "equity makes new high",
			initial:      "1000",
			updates:      []string{"1100", "990", "1200"},
			wantCurrent:  "1200",
			wantPeak:     "1200",
			wantDrawdown: "0",
		},
		{
			name:         "large drawdown - 20%",
			initial:      "1000",
			updates:      []string{"800"},
			wantCurrent:  "800",
			wantPeak:     "1000",
			wantDrawdown: "0.2", // 20%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewHighWaterMarkTracker(decimal.RequireFromString(tt.initial))

			for _, update := range tt.updates {
				tracker.Update(decimal.RequireFromString(update))
			}

			wantCurrent := decimal.RequireFromString(tt.wantCurrent)
			if !tracker.Current().Equal(wantCurrent) {
				t.Errorf("Current() = %s, want %s", tracker.Current(), wantCurrent)
			}

			wantPeak := decimal.RequireFromString(tt.wantPeak)
			if !tracker.Peak().Equal(wantPeak) {
				t.Errorf("Peak() = %s, want %s", tracker.Peak(), wantPeak)
			}

			wantDD := decimal.RequireFromString(tt.wantDrawdown)
			gotDD := tracker.Drawdown()
			// Use approximate comparison for drawdown due to decimal precision
			diff := gotDD.Sub(wantDD).Abs()
			if diff.GreaterThan(decimal.RequireFromString("0.0001")) {
				t.Errorf("Drawdown() = %s, want %s", gotDD, wantDD)
			}
		})
	}
}

func TestHighWaterMarkTracker_UpdateReturnsNewPeak(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.RequireFromString("1000"))

	// Equity increases - should return true (new peak)
	if !tracker.Update(decimal.RequireFromString("1100")) {
		t.Error("Update(1100) = false, want true (new peak)")
	}

	// Equity decreases - should return false
	if tracker.Update(decimal.RequireFromString("1050")) {
		t.Error("Update(1050) = true, want false (not new peak)")
	}

	// Same equity - should return false
	if tracker.Update(decimal.RequireFromString("1100")) {
		t.Error("Update(1100) = true, want false (equal to peak)")
	}

	// New peak again
	if !tracker.Update(decimal.RequireFromString("1200")) {
		t.Error("Update(1200) = false, want true (new peak)")
	}
}

func TestHighWaterMarkTracker_Snapshot(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.RequireFromString("1000"))
	tracker.Update(decimal.RequireFromString("1100"))
	tracker.Update(decimal.RequireFromString("990"))

	current, peak, drawdown := tracker.Snapshot()

	if !current.Equal(decimal.RequireFromString("990")) {
		t.Errorf("Snapshot current = %s, want 990", current)
	}

	if !peak.Equal(decimal.RequireFromString("1100")) {
		t.Errorf("Snapshot peak = %s, want 1100", peak)
	}

	wantDD := decimal.RequireFromString("0.1")
	diff := drawdown.Sub(wantDD).Abs()
	if diff.GreaterThan(decimal.RequireFromString("0.0001")) {
		t.Errorf("Snapshot drawdown = %s, want %s", drawdown, wantDD)
	}
}

func TestHighWaterMarkTracker_Reset(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.RequireFromString("1000"))
	tracker.Update(decimal.RequireFromString("1500"))
	tracker.Update(decimal.RequireFromString("1200"))

	// Reset to new value
	tracker.Reset(decimal.RequireFromString("2000"))

	if !tracker.Current().Equal(decimal.RequireFromString("2000")) {
		t.Errorf("After Reset, Current() = %s, want 2000", tracker.Current())
	}

	if !tracker.Peak().Equal(decimal.RequireFromString("2000")) {
		t.Errorf("After Reset, Peak() = %s, want 2000", tracker.Peak())
	}

	if !tracker.Drawdown().IsZero() {
		t.Errorf("After Reset, Drawdown() = %s, want 0", tracker.Drawdown())
	}
}

func TestHighWaterMarkTracker_ZeroPeak(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.Zero)

	// Should not panic and return zero drawdown
	dd := tracker.Drawdown()
	if !dd.IsZero() {
		t.Errorf("Drawdown with zero peak = %s, want 0", dd)
	}
}

func TestHighWaterMarkTracker_Concurrent(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.RequireFromString("1000"))

	var wg sync.WaitGroup
	numGoroutines := 100
	numUpdates := 100

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numUpdates; j++ {
				equity := decimal.NewFromInt(int64(1000 + id + j))
				tracker.Update(equity)
				_ = tracker.Current()
				_ = tracker.Peak()
				_ = tracker.Drawdown()
				_, _, _ = tracker.Snapshot()
			}
		}(i)
	}

	wg.Wait()

	// Verify state is consistent (no panic, peak >= current)
	current := tracker.Current()
	peak := tracker.Peak()
	if current.GreaterThan(peak) {
		t.Errorf("Current (%s) > Peak (%s), this should never happen", current, peak)
	}
}
