package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

func setupTestDB(t *testing.T) (*SQLiteRepository, func()) {
	t.Helper()

	// Create temp file
	f, err := os.CreateTemp("", "quant-bot-test-*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := f.Name()
	f.Close()

	repo, err := NewSQLiteRepository(path)
	if err != nil {
		os.Remove(path)
		t.Fatalf("create repository: %v", err)
	}

	cleanup := func() {
		repo.Close()
		os.Remove(path)
	}

	return repo, cleanup
}

func TestSQLiteRepository_EquitySnapshot(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Save snapshot
	snapshot := EquitySnapshot{
		Timestamp:     time.Now().Truncate(time.Second),
		Equity:        decimal.NewFromInt(10000),
		HighWaterMark: decimal.NewFromInt(10000),
		Drawdown:      decimal.Zero,
		OpenPositions: 1,
		DailyPL:       decimal.NewFromInt(100),
	}

	err := repo.SaveEquitySnapshot(ctx, snapshot)
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	// Get latest
	latest, err := repo.GetLatestEquitySnapshot(ctx)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected snapshot, got nil")
	}

	if !latest.Equity.Equal(snapshot.Equity) {
		t.Errorf("equity = %s, want %s", latest.Equity, snapshot.Equity)
	}
	if latest.OpenPositions != snapshot.OpenPositions {
		t.Errorf("open positions = %d, want %d", latest.OpenPositions, snapshot.OpenPositions)
	}
}

func TestSQLiteRepository_EquityHistory(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Save multiple snapshots
	for i := 0; i < 5; i++ {
		snapshot := EquitySnapshot{
			Timestamp:     now.Add(time.Duration(i) * time.Hour),
			Equity:        decimal.NewFromInt(int64(10000 + i*100)),
			HighWaterMark: decimal.NewFromInt(10000),
			Drawdown:      decimal.Zero,
		}
		if err := repo.SaveEquitySnapshot(ctx, snapshot); err != nil {
			t.Fatalf("save snapshot %d: %v", i, err)
		}
	}

	// Get history
	history, err := repo.GetEquityHistory(ctx, now.Add(-time.Hour), now.Add(10*time.Hour))
	if err != nil {
		t.Fatalf("get history: %v", err)
	}

	if len(history) != 5 {
		t.Errorf("history length = %d, want 5", len(history))
	}
}

func TestSQLiteRepository_Position(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Save position
	position := types.Position{
		ID:         "pos-123",
		Symbol:     "MES",
		Side:       types.SideLong,
		Contracts:  2,
		EntryPrice: decimal.NewFromInt(5000),
		EntryTime:  time.Now().Truncate(time.Second),
		StopLoss:   decimal.NewFromInt(4990),
		TakeProfit: decimal.NewFromInt(5020),
	}

	err := repo.SavePosition(ctx, position)
	if err != nil {
		t.Fatalf("save position: %v", err)
	}

	// Get open positions
	positions, err := repo.GetOpenPositions(ctx)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}

	if len(positions) != 1 {
		t.Fatalf("positions length = %d, want 1", len(positions))
	}

	if positions[0].ID != position.ID {
		t.Errorf("position ID = %s, want %s", positions[0].ID, position.ID)
	}
	if !positions[0].EntryPrice.Equal(position.EntryPrice) {
		t.Errorf("entry price = %s, want %s", positions[0].EntryPrice, position.EntryPrice)
	}

	// Close position
	exitPrice := decimal.NewFromInt(5010)
	exitTime := time.Now()
	err = repo.ClosePosition(ctx, position.ID, exitPrice, exitTime)
	if err != nil {
		t.Fatalf("close position: %v", err)
	}

	// Get open positions again
	positions, err = repo.GetOpenPositions(ctx)
	if err != nil {
		t.Fatalf("get positions after close: %v", err)
	}

	if len(positions) != 0 {
		t.Errorf("positions length after close = %d, want 0", len(positions))
	}
}

func TestSQLiteRepository_Trade(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Save trade
	trade := types.Trade{
		ID:           "trade-123",
		Symbol:       "MES",
		Side:         types.SideLong,
		Contracts:    1,
		EntryPrice:   decimal.NewFromInt(5000),
		ExitPrice:    decimal.NewFromInt(5010),
		EntryTime:    now.Add(-time.Hour),
		ExitTime:     now,
		GrossPL:      decimal.NewFromInt(50),
		Commission:   decimal.RequireFromString("1.24"),
		NetPL:        decimal.RequireFromString("48.76"),
		StrategyName: "breakout",
	}

	err := repo.SaveTrade(ctx, trade)
	if err != nil {
		t.Fatalf("save trade: %v", err)
	}

	// Get trades
	trades, err := repo.GetTrades(ctx, now.Add(-2*time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("get trades: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("trades length = %d, want 1", len(trades))
	}

	if trades[0].ID != trade.ID {
		t.Errorf("trade ID = %s, want %s", trades[0].ID, trade.ID)
	}
	if !trades[0].NetPL.Equal(trade.NetPL) {
		t.Errorf("net PL = %s, want %s", trades[0].NetPL, trade.NetPL)
	}

	// Get by symbol
	trades, err = repo.GetTradesBySymbol(ctx, "MES", 10)
	if err != nil {
		t.Fatalf("get trades by symbol: %v", err)
	}

	if len(trades) != 1 {
		t.Errorf("trades by symbol length = %d, want 1", len(trades))
	}
}

func TestSQLiteRepository_Order(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Save order
	order := OrderRecord{
		ClientOrderID: "order-123",
		Symbol:        "MES",
		Side:          types.SideLong,
		Contracts:     1,
		EntryPrice:    decimal.NewFromInt(5000),
		StopLoss:      decimal.NewFromInt(4990),
		TakeProfit:    decimal.NewFromInt(5020),
		Status:        types.OrderStatusPending,
	}

	err := repo.SaveOrder(ctx, order)
	if err != nil {
		t.Fatalf("save order: %v", err)
	}

	// Get pending orders
	orders, err := repo.GetPendingOrders(ctx)
	if err != nil {
		t.Fatalf("get pending orders: %v", err)
	}

	if len(orders) != 1 {
		t.Fatalf("pending orders length = %d, want 1", len(orders))
	}

	if orders[0].ClientOrderID != order.ClientOrderID {
		t.Errorf("client order ID = %s, want %s", orders[0].ClientOrderID, order.ClientOrderID)
	}

	// Update order status to filled
	fillPrice := decimal.NewFromInt(5001)
	err = repo.UpdateOrderStatus(ctx, order.ClientOrderID, types.OrderStatusFilled, fillPrice)
	if err != nil {
		t.Fatalf("update order status: %v", err)
	}

	// Get pending orders again (should be empty)
	orders, err = repo.GetPendingOrders(ctx)
	if err != nil {
		t.Fatalf("get pending orders after fill: %v", err)
	}

	if len(orders) != 0 {
		t.Errorf("pending orders after fill = %d, want 0", len(orders))
	}
}

func TestSQLiteRepository_BotState(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Initially no state
	state, err := repo.GetState(ctx)
	if err != nil {
		t.Fatalf("get initial state: %v", err)
	}
	if state != nil {
		t.Error("expected nil state initially")
	}

	// Save state
	newState := BotState{
		LastUpdated:      time.Now().Truncate(time.Second),
		Equity:           decimal.NewFromInt(10500),
		HighWaterMark:    decimal.NewFromInt(11000),
		KillSwitchActive: false,
		SafeModeActive:   false,
		TotalTrades:      10,
		WinningTrades:    6,
		LosingTrades:     4,
		TotalPL:          decimal.NewFromInt(500),
	}

	err = repo.SaveState(ctx, newState)
	if err != nil {
		t.Fatalf("save state: %v", err)
	}

	// Get state
	state, err = repo.GetState(ctx)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state == nil {
		t.Fatal("expected state, got nil")
	}

	if !state.Equity.Equal(newState.Equity) {
		t.Errorf("equity = %s, want %s", state.Equity, newState.Equity)
	}
	if state.TotalTrades != newState.TotalTrades {
		t.Errorf("total trades = %d, want %d", state.TotalTrades, newState.TotalTrades)
	}
	if state.KillSwitchActive != newState.KillSwitchActive {
		t.Errorf("kill switch = %v, want %v", state.KillSwitchActive, newState.KillSwitchActive)
	}

	// Update state (upsert)
	newState.Equity = decimal.NewFromInt(11000)
	newState.KillSwitchActive = true
	err = repo.SaveState(ctx, newState)
	if err != nil {
		t.Fatalf("update state: %v", err)
	}

	state, err = repo.GetState(ctx)
	if err != nil {
		t.Fatalf("get updated state: %v", err)
	}

	if !state.Equity.Equal(decimal.NewFromInt(11000)) {
		t.Errorf("updated equity = %s, want 11000", state.Equity)
	}
	if !state.KillSwitchActive {
		t.Error("kill switch should be active")
	}
}

func TestSQLiteRepository_NoData(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// No equity snapshot
	snapshot, err := repo.GetLatestEquitySnapshot(ctx)
	if err != nil {
		t.Fatalf("get latest snapshot: %v", err)
	}
	if snapshot != nil {
		t.Error("expected nil snapshot")
	}

	// No positions
	positions, err := repo.GetOpenPositions(ctx)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions = %d, want 0", len(positions))
	}

	// No trades
	trades, err := repo.GetTrades(ctx, time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("get trades: %v", err)
	}
	if len(trades) != 0 {
		t.Errorf("trades = %d, want 0", len(trades))
	}

	// No pending orders
	orders, err := repo.GetPendingOrders(ctx)
	if err != nil {
		t.Fatalf("get pending orders: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("pending orders = %d, want 0", len(orders))
	}
}
