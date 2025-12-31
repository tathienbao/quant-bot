package risk

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// TestEngine_Concurrent_UpdateEquity tests concurrent equity updates (RACE-01).
// 100 goroutines updating equity should not cause data corruption.
func TestEngine_Concurrent_UpdateEquity(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.NewFromInt(10000), nil)

	var wg sync.WaitGroup
	numGoroutines := 100
	updatesPerGoroutine := 100

	// Launch concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				// Vary equity up and down
				equity := decimal.NewFromInt(int64(10000 + (id*j)%1000 - 500))
				engine.UpdateEquity(equity)
			}
		}(i)
	}

	wg.Wait()

	// Verify no panic and engine is still usable
	snapshot := engine.GetSnapshot()
	if snapshot.Equity.IsZero() && snapshot.HighWaterMark.IsZero() {
		t.Error("expected non-zero values after concurrent updates")
	}

	// HWM should be >= equity (invariant)
	if snapshot.HighWaterMark.LessThan(snapshot.Equity) {
		t.Error("HWM invariant violated after concurrent updates")
	}
}

// TestEngine_Concurrent_ValidateNearKillSwitch tests atomic check near threshold (RACE-02).
func TestEngine_Concurrent_ValidateNearKillSwitch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxGlobalDrawdownPct = decimal.RequireFromString("0.20")
	engine := NewEngine(cfg, decimal.NewFromInt(10000), nil)

	// Set equity near threshold (19% DD)
	engine.UpdateEquity(decimal.NewFromInt(8100))

	var wg sync.WaitGroup
	numSignals := 50

	signal := types.Signal{
		ID:        "race-signal",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	}

	event := types.MarketEvent{
		Symbol: "MES",
		Close:  decimal.NewFromInt(5000),
		ATR:    decimal.NewFromInt(10),
	}

	// Concurrent signal validation
	for i := 0; i < numSignals; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sig := signal
			sig.ID = string(rune('A' + id%26))
			_, _ = engine.ValidateAndSize(context.Background(), sig, event)
		}(i)
	}

	wg.Wait()

	// No specific assertion - just verify no panic/deadlock
}

// TestEngine_Concurrent_GetSnapshotDuringUpdate tests consistent snapshot (RACE-03).
func TestEngine_Concurrent_GetSnapshotDuringUpdate(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.NewFromInt(10000), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				equity := decimal.NewFromInt(int64(10000 + i%500))
				engine.UpdateEquity(equity)
			}
		}
	}()

	// Reader goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					snapshot := engine.GetSnapshot()
					// Verify snapshot consistency
					if snapshot.HighWaterMark.LessThan(snapshot.Equity) {
						t.Error("inconsistent snapshot: HWM < Equity")
					}
					if snapshot.Drawdown.LessThan(decimal.Zero) {
						t.Error("inconsistent snapshot: negative drawdown")
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestEngine_Concurrent_PositionUpdates tests position update thread safety.
func TestEngine_Concurrent_PositionUpdates(t *testing.T) {
	cfg := DefaultConfig()
	engine := NewEngine(cfg, decimal.NewFromInt(10000), nil)

	var wg sync.WaitGroup
	numGoroutines := 20

	// Concurrent position updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				pos := &types.Position{
					ID:         "pos-concurrent",
					Symbol:     "MES",
					Side:       types.SideLong,
					Contracts:  id%5 + 1,
					EntryPrice: decimal.NewFromInt(5000),
				}
				engine.UpdatePosition(pos)
				_, _ = engine.GetPosition("MES")
			}
		}(i)
	}

	wg.Wait()
	// No specific assertion - verify no panic
}

// TestHighWaterMarkTracker_Concurrent tests HWM tracker thread safety.
func TestHighWaterMarkTracker_Concurrent_Updates(t *testing.T) {
	tracker := NewHighWaterMarkTracker(decimal.NewFromInt(10000))

	var wg sync.WaitGroup
	numGoroutines := 100
	updatesPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				equity := decimal.NewFromInt(int64(10000 + id*10 + j))
				tracker.Update(equity)
				_, _, _ = tracker.Snapshot()
			}
		}(i)
	}

	wg.Wait()

	// Final snapshot should be consistent
	current, peak, _ := tracker.Snapshot()
	if peak.LessThan(current) {
		t.Error("HWM invariant violated")
	}
}
