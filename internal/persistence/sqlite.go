package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteRepository implements Repository using SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite repository.
func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	repo := &SQLiteRepository{db: db}

	// Run migrations
	if err := repo.Migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return repo, nil
}

// Migrate runs database migrations.
func (r *SQLiteRepository) Migrate(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS equity_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			equity TEXT NOT NULL,
			high_water_mark TEXT NOT NULL,
			drawdown TEXT NOT NULL,
			open_positions INTEGER NOT NULL DEFAULT 0,
			daily_pl TEXT NOT NULL DEFAULT '0',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_equity_timestamp ON equity_snapshots(timestamp)`,

		`CREATE TABLE IF NOT EXISTS positions (
			id TEXT PRIMARY KEY,
			symbol TEXT NOT NULL,
			side INTEGER NOT NULL,
			contracts INTEGER NOT NULL,
			entry_price TEXT NOT NULL,
			entry_time DATETIME NOT NULL,
			stop_loss TEXT,
			take_profit TEXT,
			exit_price TEXT,
			exit_time DATETIME,
			unrealized_pl TEXT NOT NULL DEFAULT '0',
			realized_pl TEXT NOT NULL DEFAULT '0',
			is_open INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_positions_symbol ON positions(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_positions_is_open ON positions(is_open)`,

		`CREATE TABLE IF NOT EXISTS trades (
			id TEXT PRIMARY KEY,
			symbol TEXT NOT NULL,
			side INTEGER NOT NULL,
			contracts INTEGER NOT NULL,
			entry_price TEXT NOT NULL,
			exit_price TEXT NOT NULL,
			entry_time DATETIME NOT NULL,
			exit_time DATETIME NOT NULL,
			gross_pl TEXT NOT NULL,
			commission TEXT NOT NULL,
			net_pl TEXT NOT NULL,
			r_multiple TEXT,
			signal_id TEXT,
			strategy_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades(symbol)`,
		`CREATE INDEX IF NOT EXISTS idx_trades_exit_time ON trades(exit_time)`,

		`CREATE TABLE IF NOT EXISTS orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_order_id TEXT UNIQUE NOT NULL,
			symbol TEXT NOT NULL,
			side INTEGER NOT NULL,
			contracts INTEGER NOT NULL,
			entry_price TEXT NOT NULL,
			stop_loss TEXT,
			take_profit TEXT,
			status INTEGER NOT NULL,
			filled_price TEXT,
			filled_at DATETIME,
			signal_id TEXT,
			strategy_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_client_order_id ON orders(client_order_id)`,
		`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,

		`CREATE TABLE IF NOT EXISTS bot_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			last_updated DATETIME NOT NULL,
			equity TEXT NOT NULL,
			high_water_mark TEXT NOT NULL,
			kill_switch_active INTEGER NOT NULL DEFAULT 0,
			safe_mode_active INTEGER NOT NULL DEFAULT 0,
			total_trades INTEGER NOT NULL DEFAULT 0,
			winning_trades INTEGER NOT NULL DEFAULT 0,
			losing_trades INTEGER NOT NULL DEFAULT 0,
			total_pl TEXT NOT NULL DEFAULT '0'
		)`,
	}

	for _, migration := range migrations {
		if _, err := r.db.ExecContext(ctx, migration); err != nil {
			return fmt.Errorf("execute migration: %w", err)
		}
	}

	return nil
}

// SaveEquitySnapshot saves an equity snapshot.
func (r *SQLiteRepository) SaveEquitySnapshot(ctx context.Context, snapshot EquitySnapshot) error {
	query := `INSERT INTO equity_snapshots (timestamp, equity, high_water_mark, drawdown, open_positions, daily_pl)
		VALUES (?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		snapshot.Timestamp,
		snapshot.Equity.String(),
		snapshot.HighWaterMark.String(),
		snapshot.Drawdown.String(),
		snapshot.OpenPositions,
		snapshot.DailyPL.String(),
	)
	if err != nil {
		return fmt.Errorf("insert equity snapshot: %w", err)
	}

	return nil
}

// GetLatestEquitySnapshot returns the most recent equity snapshot.
func (r *SQLiteRepository) GetLatestEquitySnapshot(ctx context.Context) (*EquitySnapshot, error) {
	query := `SELECT id, timestamp, equity, high_water_mark, drawdown, open_positions, daily_pl
		FROM equity_snapshots ORDER BY timestamp DESC LIMIT 1`

	var snapshot EquitySnapshot
	var equity, hwm, dd, dailyPL string

	err := r.db.QueryRowContext(ctx, query).Scan(
		&snapshot.ID,
		&snapshot.Timestamp,
		&equity,
		&hwm,
		&dd,
		&snapshot.OpenPositions,
		&dailyPL,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query equity snapshot: %w", err)
	}

	snapshot.Equity, _ = decimal.NewFromString(equity)
	snapshot.HighWaterMark, _ = decimal.NewFromString(hwm)
	snapshot.Drawdown, _ = decimal.NewFromString(dd)
	snapshot.DailyPL, _ = decimal.NewFromString(dailyPL)

	return &snapshot, nil
}

// GetEquityHistory returns equity snapshots in a time range.
func (r *SQLiteRepository) GetEquityHistory(ctx context.Context, from, to time.Time) ([]EquitySnapshot, error) {
	query := `SELECT id, timestamp, equity, high_water_mark, drawdown, open_positions, daily_pl
		FROM equity_snapshots WHERE timestamp BETWEEN ? AND ? ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("query equity history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var snapshots []EquitySnapshot
	for rows.Next() {
		var s EquitySnapshot
		var equity, hwm, dd, dailyPL string

		if err := rows.Scan(&s.ID, &s.Timestamp, &equity, &hwm, &dd, &s.OpenPositions, &dailyPL); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		s.Equity, _ = decimal.NewFromString(equity)
		s.HighWaterMark, _ = decimal.NewFromString(hwm)
		s.Drawdown, _ = decimal.NewFromString(dd)
		s.DailyPL, _ = decimal.NewFromString(dailyPL)

		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}

// SavePosition saves a position.
func (r *SQLiteRepository) SavePosition(ctx context.Context, position types.Position) error {
	query := `INSERT OR REPLACE INTO positions
		(id, symbol, side, contracts, entry_price, entry_time, stop_loss, take_profit, unrealized_pl, realized_pl, is_open, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP)`

	_, err := r.db.ExecContext(ctx, query,
		position.ID,
		position.Symbol,
		position.Side,
		position.Contracts,
		position.EntryPrice.String(),
		position.EntryTime,
		position.StopLoss.String(),
		position.TakeProfit.String(),
		position.UnrealizedPL.String(),
		position.RealizedPL.String(),
	)
	if err != nil {
		return fmt.Errorf("insert position: %w", err)
	}

	return nil
}

// GetOpenPositions returns all open positions.
func (r *SQLiteRepository) GetOpenPositions(ctx context.Context) ([]types.Position, error) {
	query := `SELECT id, symbol, side, contracts, entry_price, entry_time, stop_loss, take_profit, unrealized_pl, realized_pl
		FROM positions WHERE is_open = 1`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query positions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var positions []types.Position
	for rows.Next() {
		var p types.Position
		var entryPrice, stopLoss, takeProfit, unrealizedPL, realizedPL string

		if err := rows.Scan(&p.ID, &p.Symbol, &p.Side, &p.Contracts, &entryPrice, &p.EntryTime, &stopLoss, &takeProfit, &unrealizedPL, &realizedPL); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		p.EntryPrice, _ = decimal.NewFromString(entryPrice)
		p.StopLoss, _ = decimal.NewFromString(stopLoss)
		p.TakeProfit, _ = decimal.NewFromString(takeProfit)
		p.UnrealizedPL, _ = decimal.NewFromString(unrealizedPL)
		p.RealizedPL, _ = decimal.NewFromString(realizedPL)

		positions = append(positions, p)
	}

	return positions, rows.Err()
}

// ClosePosition marks a position as closed.
func (r *SQLiteRepository) ClosePosition(ctx context.Context, positionID string, exitPrice decimal.Decimal, exitTime time.Time) error {
	query := `UPDATE positions SET is_open = 0, exit_price = ?, exit_time = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, exitPrice.String(), exitTime, positionID)
	if err != nil {
		return fmt.Errorf("close position: %w", err)
	}

	return nil
}

// SaveTrade saves a completed trade.
func (r *SQLiteRepository) SaveTrade(ctx context.Context, trade types.Trade) error {
	query := `INSERT INTO trades
		(id, symbol, side, contracts, entry_price, exit_price, entry_time, exit_time, gross_pl, commission, net_pl, r_multiple, signal_id, strategy_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		trade.ID,
		trade.Symbol,
		trade.Side,
		trade.Contracts,
		trade.EntryPrice.String(),
		trade.ExitPrice.String(),
		trade.EntryTime,
		trade.ExitTime,
		trade.GrossPL.String(),
		trade.Commission.String(),
		trade.NetPL.String(),
		trade.RMultiple.String(),
		trade.SignalID,
		trade.StrategyName,
	)
	if err != nil {
		return fmt.Errorf("insert trade: %w", err)
	}

	return nil
}

// GetTrades returns trades in a time range.
func (r *SQLiteRepository) GetTrades(ctx context.Context, from, to time.Time) ([]types.Trade, error) {
	query := `SELECT id, symbol, side, contracts, entry_price, exit_price, entry_time, exit_time, gross_pl, commission, net_pl, r_multiple, signal_id, strategy_name
		FROM trades WHERE exit_time BETWEEN ? AND ? ORDER BY exit_time DESC`

	rows, err := r.db.QueryContext(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("query trades: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTrades(rows)
}

// GetTradesBySymbol returns trades for a symbol.
func (r *SQLiteRepository) GetTradesBySymbol(ctx context.Context, symbol string, limit int) ([]types.Trade, error) {
	query := `SELECT id, symbol, side, contracts, entry_price, exit_price, entry_time, exit_time, gross_pl, commission, net_pl, r_multiple, signal_id, strategy_name
		FROM trades WHERE symbol = ? ORDER BY exit_time DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("query trades by symbol: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTrades(rows)
}

func (r *SQLiteRepository) scanTrades(rows *sql.Rows) ([]types.Trade, error) {
	var trades []types.Trade
	for rows.Next() {
		var t types.Trade
		var entryPrice, exitPrice, grossPL, commission, netPL, rMultiple string
		var signalID, strategyName sql.NullString

		if err := rows.Scan(&t.ID, &t.Symbol, &t.Side, &t.Contracts, &entryPrice, &exitPrice, &t.EntryTime, &t.ExitTime, &grossPL, &commission, &netPL, &rMultiple, &signalID, &strategyName); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		t.EntryPrice, _ = decimal.NewFromString(entryPrice)
		t.ExitPrice, _ = decimal.NewFromString(exitPrice)
		t.GrossPL, _ = decimal.NewFromString(grossPL)
		t.Commission, _ = decimal.NewFromString(commission)
		t.NetPL, _ = decimal.NewFromString(netPL)
		t.RMultiple, _ = decimal.NewFromString(rMultiple)
		t.SignalID = signalID.String
		t.StrategyName = strategyName.String

		trades = append(trades, t)
	}

	return trades, rows.Err()
}

// SaveOrder saves an order.
func (r *SQLiteRepository) SaveOrder(ctx context.Context, order OrderRecord) error {
	query := `INSERT INTO orders
		(client_order_id, symbol, side, contracts, entry_price, stop_loss, take_profit, status, signal_id, strategy_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		order.ClientOrderID,
		order.Symbol,
		order.Side,
		order.Contracts,
		order.EntryPrice.String(),
		order.StopLoss.String(),
		order.TakeProfit.String(),
		order.Status,
		order.SignalID,
		order.StrategyName,
	)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	return nil
}

// GetPendingOrders returns orders with non-final status.
func (r *SQLiteRepository) GetPendingOrders(ctx context.Context) ([]OrderRecord, error) {
	query := `SELECT id, client_order_id, symbol, side, contracts, entry_price, stop_loss, take_profit, status, filled_price, filled_at, signal_id, strategy_name, created_at, updated_at
		FROM orders WHERE status < ?`

	rows, err := r.db.QueryContext(ctx, query, types.OrderStatusFilled)
	if err != nil {
		return nil, fmt.Errorf("query pending orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var orders []OrderRecord
	for rows.Next() {
		var o OrderRecord
		var entryPrice, stopLoss, takeProfit string
		var filledPrice sql.NullString
		var filledAt sql.NullTime
		var signalID, strategyName sql.NullString

		if err := rows.Scan(&o.ID, &o.ClientOrderID, &o.Symbol, &o.Side, &o.Contracts, &entryPrice, &stopLoss, &takeProfit, &o.Status, &filledPrice, &filledAt, &signalID, &strategyName, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		o.EntryPrice, _ = decimal.NewFromString(entryPrice)
		o.StopLoss, _ = decimal.NewFromString(stopLoss)
		o.TakeProfit, _ = decimal.NewFromString(takeProfit)
		if filledPrice.Valid {
			o.FilledPrice, _ = decimal.NewFromString(filledPrice.String)
		}
		if filledAt.Valid {
			o.FilledAt = &filledAt.Time
		}
		o.SignalID = signalID.String
		o.StrategyName = strategyName.String

		orders = append(orders, o)
	}

	return orders, rows.Err()
}

// UpdateOrderStatus updates an order's status.
func (r *SQLiteRepository) UpdateOrderStatus(ctx context.Context, clientOrderID string, status types.OrderStatus, fillPrice decimal.Decimal) error {
	var query string
	var args []interface{}

	if status == types.OrderStatusFilled {
		query = `UPDATE orders SET status = ?, filled_price = ?, filled_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE client_order_id = ?`
		args = []interface{}{status, fillPrice.String(), clientOrderID}
	} else {
		query = `UPDATE orders SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE client_order_id = ?`
		args = []interface{}{status, clientOrderID}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}

	return nil
}

// SaveState saves the bot state.
func (r *SQLiteRepository) SaveState(ctx context.Context, state BotState) error {
	query := `INSERT OR REPLACE INTO bot_state
		(id, last_updated, equity, high_water_mark, kill_switch_active, safe_mode_active, total_trades, winning_trades, losing_trades, total_pl)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		state.LastUpdated,
		state.Equity.String(),
		state.HighWaterMark.String(),
		boolToInt(state.KillSwitchActive),
		boolToInt(state.SafeModeActive),
		state.TotalTrades,
		state.WinningTrades,
		state.LosingTrades,
		state.TotalPL.String(),
	)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}

// GetState returns the saved bot state.
func (r *SQLiteRepository) GetState(ctx context.Context) (*BotState, error) {
	query := `SELECT id, last_updated, equity, high_water_mark, kill_switch_active, safe_mode_active, total_trades, winning_trades, losing_trades, total_pl
		FROM bot_state WHERE id = 1`

	var state BotState
	var equity, hwm, totalPL string
	var killSwitch, safeMode int

	err := r.db.QueryRowContext(ctx, query).Scan(
		&state.ID,
		&state.LastUpdated,
		&equity,
		&hwm,
		&killSwitch,
		&safeMode,
		&state.TotalTrades,
		&state.WinningTrades,
		&state.LosingTrades,
		&totalPL,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query state: %w", err)
	}

	state.Equity, _ = decimal.NewFromString(equity)
	state.HighWaterMark, _ = decimal.NewFromString(hwm)
	state.TotalPL, _ = decimal.NewFromString(totalPL)
	state.KillSwitchActive = killSwitch == 1
	state.SafeModeActive = safeMode == 1

	return &state, nil
}

// Close closes the database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
