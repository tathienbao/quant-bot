package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// TestRecovery_StateRestored tests state restoration on startup (SHUT-01).
func TestRecovery_StateRestored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Create first repository and save state
	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	if err := repo1.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Save state
	originalState := BotState{
		LastUpdated:      time.Now(),
		Equity:           decimal.NewFromInt(12500),
		HighWaterMark:    decimal.NewFromInt(15000),
		KillSwitchActive: false,
		SafeModeActive:   false,
		TotalTrades:      42,
		WinningTrades:    25,
		LosingTrades:     17,
		TotalPL:          decimal.NewFromInt(2500),
	}

	if err := repo1.SaveState(ctx, originalState); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	repo1.Close()

	// Create second repository (simulating restart)
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	// Restore state
	restoredState, err := repo2.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	// Verify restored values
	if !restoredState.Equity.Equal(originalState.Equity) {
		t.Errorf("equity mismatch: got %s, want %s", restoredState.Equity, originalState.Equity)
	}

	if !restoredState.HighWaterMark.Equal(originalState.HighWaterMark) {
		t.Errorf("HWM mismatch: got %s, want %s", restoredState.HighWaterMark, originalState.HighWaterMark)
	}

	if restoredState.TotalTrades != originalState.TotalTrades {
		t.Errorf("total trades mismatch: got %d, want %d", restoredState.TotalTrades, originalState.TotalTrades)
	}
}

// TestRecovery_KillSwitchPreserved tests kill switch state preservation (SHUT-02).
func TestRecovery_KillSwitchPreserved(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_ks_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	// Create and save state with kill switch active
	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	if err := repo1.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	state := BotState{
		LastUpdated:      time.Now(),
		Equity:           decimal.NewFromInt(7500),
		HighWaterMark:    decimal.NewFromInt(10000),
		KillSwitchActive: true, // Kill switch was active
		SafeModeActive:   true,
		TotalPL:          decimal.NewFromInt(-2500),
	}

	if err := repo1.SaveState(ctx, state); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
	repo1.Close()

	// Restore
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	restored, err := repo2.GetState(ctx)
	if err != nil {
		t.Fatalf("failed to get state: %v", err)
	}

	// Kill switch should remain active after restart
	if !restored.KillSwitchActive {
		t.Error("kill switch should remain active after restart")
	}

	if !restored.SafeModeActive {
		t.Error("safe mode should remain active after restart")
	}
}

// TestRecovery_OpenPositionsRestored tests open positions restoration (SHUT-03).
func TestRecovery_OpenPositionsRestored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_pos_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	if err := repo1.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Save open positions
	positions := []types.Position{
		{
			ID:         "pos-1",
			Symbol:     "MES",
			Side:       types.SideLong,
			Contracts:  2,
			EntryPrice: decimal.NewFromInt(5000),
			EntryTime:  time.Now().Add(-1 * time.Hour),
		},
		{
			ID:         "pos-2",
			Symbol:     "MGC",
			Side:       types.SideShort,
			Contracts:  1,
			EntryPrice: decimal.NewFromInt(2000),
			EntryTime:  time.Now().Add(-30 * time.Minute),
		},
	}

	for _, pos := range positions {
		if err := repo1.SavePosition(ctx, pos); err != nil {
			t.Fatalf("failed to save position: %v", err)
		}
	}
	repo1.Close()

	// Restore
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	restored, err := repo2.GetOpenPositions(ctx)
	if err != nil {
		t.Fatalf("failed to get positions: %v", err)
	}

	if len(restored) != len(positions) {
		t.Fatalf("position count mismatch: got %d, want %d", len(restored), len(positions))
	}

	// Check MES position
	var mesPos *types.Position
	for i := range restored {
		if restored[i].Symbol == "MES" {
			mesPos = &restored[i]
			break
		}
	}

	if mesPos == nil {
		t.Fatal("MES position not found")
	}

	if mesPos.Contracts != 2 {
		t.Errorf("MES contracts mismatch: got %d, want 2", mesPos.Contracts)
	}
}

// TestRecovery_PendingOrdersRestored tests pending orders restoration (SHUT-04).
func TestRecovery_PendingOrdersRestored(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_ord_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	if err := repo1.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Save pending orders
	orders := []OrderRecord{
		{
			ClientOrderID: "order-1",
			Symbol:        "MES",
			Side:          types.SideLong,
			Contracts:     2,
			EntryPrice:    decimal.NewFromInt(5000),
			StopLoss:      decimal.NewFromInt(4990),
			TakeProfit:    decimal.NewFromInt(5015),
			Status:        types.OrderStatusPending,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
		{
			ClientOrderID: "order-2",
			Symbol:        "MES",
			Side:          types.SideShort,
			Contracts:     1,
			EntryPrice:    decimal.NewFromInt(5010),
			StopLoss:      decimal.NewFromInt(5020),
			TakeProfit:    decimal.NewFromInt(4995),
			Status:        types.OrderStatusPending,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		},
	}

	for _, order := range orders {
		if err := repo1.SaveOrder(ctx, order); err != nil {
			t.Fatalf("failed to save order: %v", err)
		}
	}
	repo1.Close()

	// Restore
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	restored, err := repo2.GetPendingOrders(ctx)
	if err != nil {
		t.Fatalf("failed to get pending orders: %v", err)
	}

	if len(restored) != 2 {
		t.Errorf("pending order count mismatch: got %d, want 2", len(restored))
	}
}

// TestRecovery_EquityHistoryPreserved tests equity history preservation (SHUT-05).
func TestRecovery_EquityHistoryPreserved(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_eq_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	repo1, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	if err := repo1.Migrate(ctx); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Save equity snapshots
	baseTime := time.Now().Add(-1 * time.Hour)
	snapshots := []EquitySnapshot{
		{
			Timestamp:     baseTime,
			Equity:        decimal.NewFromInt(10000),
			HighWaterMark: decimal.NewFromInt(10000),
			Drawdown:      decimal.Zero,
		},
		{
			Timestamp:     baseTime.Add(20 * time.Minute),
			Equity:        decimal.NewFromInt(10500),
			HighWaterMark: decimal.NewFromInt(10500),
			Drawdown:      decimal.Zero,
		},
		{
			Timestamp:     baseTime.Add(40 * time.Minute),
			Equity:        decimal.NewFromInt(10200),
			HighWaterMark: decimal.NewFromInt(10500),
			Drawdown:      decimal.RequireFromString("0.0286"),
		},
	}

	for _, snap := range snapshots {
		if err := repo1.SaveEquitySnapshot(ctx, snap); err != nil {
			t.Fatalf("failed to save snapshot: %v", err)
		}
	}
	repo1.Close()

	// Restore
	repo2, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create second repository: %v", err)
	}
	defer repo2.Close()

	// Get equity history
	history, err := repo2.GetEquityHistory(ctx, baseTime.Add(-1*time.Minute), time.Now())
	if err != nil {
		t.Fatalf("failed to get equity history: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("equity history count mismatch: got %d, want 3", len(history))
	}

	// Get latest snapshot
	latest, err := repo2.GetLatestEquitySnapshot(ctx)
	if err != nil {
		t.Fatalf("failed to get latest snapshot: %v", err)
	}

	// Latest should be the last one saved
	if !latest.Equity.Equal(decimal.NewFromInt(10200)) {
		t.Errorf("latest equity mismatch: got %s, want 10200", latest.Equity)
	}
}
