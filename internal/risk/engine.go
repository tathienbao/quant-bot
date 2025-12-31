package risk

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Config holds the risk engine configuration.
type Config struct {
	MaxGlobalDrawdownPct    decimal.Decimal // e.g., 0.20 for 20%
	RiskPerTradePct         decimal.Decimal // e.g., 0.01 for 1%
	MaxExposurePerSymbolPct decimal.Decimal // e.g., 0.50 for 50%
	MaxTotalExposurePct     decimal.Decimal // e.g., 1.00 for 100%
	StopLossATRMultiple     decimal.Decimal // e.g., 2.0
	TakeProfitATRMultiple   decimal.Decimal // e.g., 3.0
}

// DefaultConfig returns a conservative default configuration.
func DefaultConfig() Config {
	return Config{
		MaxGlobalDrawdownPct:    decimal.RequireFromString("0.20"),
		RiskPerTradePct:         decimal.RequireFromString("0.01"),
		MaxExposurePerSymbolPct: decimal.RequireFromString("0.50"),
		MaxTotalExposurePct:     decimal.RequireFromString("1.00"),
		StopLossATRMultiple:     decimal.RequireFromString("2.0"),
		TakeProfitATRMultiple:   decimal.RequireFromString("3.0"),
	}
}

// Engine is the main risk management engine.
// It validates signals, calculates position sizes, and enforces risk limits.
// Thread-safe for concurrent access.
type Engine struct {
	mu sync.RWMutex

	cfg       Config
	hwm       *HighWaterMarkTracker
	sizers    map[string]*PositionSizer // symbol -> sizer
	positions map[string]*types.Position // symbol -> position

	safeMode   bool
	safeModeAt time.Time

	logger *slog.Logger
}

// NewEngine creates a new risk engine.
func NewEngine(cfg Config, initialEquity decimal.Decimal, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	return &Engine{
		cfg:       cfg,
		hwm:       NewHighWaterMarkTracker(initialEquity),
		sizers:    make(map[string]*PositionSizer),
		positions: make(map[string]*types.Position),
		logger:    logger,
	}
}

// ValidateAndSize validates a signal and returns an OrderIntent if approved.
// Returns an error if the signal is rejected.
func (e *Engine) ValidateAndSize(ctx context.Context, signal types.Signal, marketEvent types.MarketEvent) (*types.OrderIntent, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Check safe mode
	if e.safeMode {
		e.logger.Warn("signal rejected: safe mode active",
			"signal_id", signal.ID,
			"symbol", signal.Symbol,
		)
		return nil, types.ErrKillSwitchActive
	}

	// Check drawdown - if already in drawdown territory, enter safe mode first
	drawdown := e.hwm.Drawdown()
	if drawdown.GreaterThanOrEqual(e.cfg.MaxGlobalDrawdownPct) {
		if !e.safeMode {
			e.enterSafeModeLocked("max drawdown exceeded")
		}
		return nil, types.ErrKillSwitchActive
	}

	// Get or create sizer for symbol
	sizer, err := e.getOrCreateSizer(signal.Symbol)
	if err != nil {
		return nil, fmt.Errorf("create sizer: %w", err)
	}

	// Get instrument spec
	spec, ok := types.GetInstrumentSpec(signal.Symbol)
	if !ok {
		return nil, types.ErrInvalidSymbol
	}

	// Calculate stop distance in ticks
	stopTicks := signal.StopTicks
	if stopTicks <= 0 {
		// Use ATR-based stop if not specified
		if marketEvent.ATR.IsZero() {
			return nil, fmt.Errorf("no stop distance and ATR unavailable")
		}
		atrStop := marketEvent.ATR.Mul(e.cfg.StopLossATRMultiple)
		stopTicks = int(atrStop.Div(spec.TickSize).Ceil().IntPart())
	}

	// Calculate position size
	equity := e.hwm.Current()
	result := sizer.CalculateWithDetails(
		equity,
		e.cfg.RiskPerTradePct,
		stopTicks,
		marketEvent.Close,
		signal.Direction,
		spec.TickSize,
	)

	if !result.Valid {
		e.logger.Info("signal rejected: position sizing failed",
			"signal_id", signal.ID,
			"reason", result.RejectReason,
		)
		return nil, fmt.Errorf("%w: %s", types.ErrInsufficientEquity, result.RejectReason)
	}

	// Check exposure limits
	if err := e.checkExposureLimits(signal.Symbol, result.Contracts, marketEvent.Close, spec); err != nil {
		e.logger.Info("signal rejected: exposure limit",
			"signal_id", signal.ID,
			"error", err,
		)
		return nil, err
	}

	// Calculate take profit
	var takeProfit decimal.Decimal
	tpDistance := spec.TickSize.Mul(decimal.NewFromInt(int64(stopTicks))).Mul(e.cfg.TakeProfitATRMultiple.Div(e.cfg.StopLossATRMultiple))
	switch signal.Direction {
	case types.SideLong:
		takeProfit = marketEvent.Close.Add(tpDistance)
	case types.SideShort:
		takeProfit = marketEvent.Close.Sub(tpDistance)
	}

	// Create order intent
	intent := &types.OrderIntent{
		ID:              uuid.New().String(),
		ClientOrderID:   generateClientOrderID(),
		Timestamp:       time.Now(),
		Symbol:          signal.Symbol,
		Side:            signal.Direction,
		Contracts:       result.Contracts,
		EntryPrice:      marketEvent.Close,
		StopLoss:        result.StopLoss,
		TakeProfit:      takeProfit,
		RiskAmount:      result.RiskAmount,
		SignalID:        signal.ID,
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	e.logger.Info("order intent created",
		"order_id", intent.ID,
		"client_order_id", intent.ClientOrderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"contracts", intent.Contracts,
		"entry", intent.EntryPrice,
		"stop_loss", intent.StopLoss,
		"take_profit", intent.TakeProfit,
		"risk_amount", intent.RiskAmount,
	)

	return intent, nil
}

// UpdateEquity updates the current equity and checks for drawdown limits.
func (e *Engine) UpdateEquity(equity decimal.Decimal) {
	e.mu.Lock()
	defer e.mu.Unlock()

	newPeak := e.hwm.Update(equity)

	if newPeak {
		e.logger.Info("new equity peak",
			"equity", equity,
		)
	}

	// Check drawdown
	drawdown := e.hwm.Drawdown()
	if drawdown.GreaterThanOrEqual(e.cfg.MaxGlobalDrawdownPct) {
		e.enterSafeModeLocked("max drawdown exceeded")
	}
}

// UpdatePosition updates or creates a position.
func (e *Engine) UpdatePosition(position *types.Position) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if position.Contracts == 0 {
		delete(e.positions, position.Symbol)
	} else {
		e.positions[position.Symbol] = position
	}
}

// GetPosition returns the current position for a symbol.
func (e *Engine) GetPosition(symbol string) (*types.Position, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	pos, ok := e.positions[symbol]
	return pos, ok
}

// IsInSafeMode returns true if the engine is in safe mode.
func (e *Engine) IsInSafeMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.safeMode
}

// EnterSafeMode manually enters safe mode.
func (e *Engine) EnterSafeMode(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enterSafeModeLocked(reason)
}

// ExitSafeMode exits safe mode (manual reset).
func (e *Engine) ExitSafeMode() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.safeMode {
		e.safeMode = false
		e.logger.Warn("safe mode exited manually")
	}
}

// GetSnapshot returns the current state.
func (e *Engine) GetSnapshot() types.EquitySnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	current, peak, drawdown := e.hwm.Snapshot()

	return types.EquitySnapshot{
		Timestamp:     time.Now(),
		Equity:        current,
		HighWaterMark: peak,
		Drawdown:      drawdown,
		OpenPositions: len(e.positions),
	}
}

// Shutdown gracefully shuts down the engine.
func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info("risk engine shutting down",
		"open_positions", len(e.positions),
		"safe_mode", e.safeMode,
	)

	// Nothing async to wait for currently
	return nil
}

// enterSafeModeLocked enters safe mode. Must be called with lock held.
func (e *Engine) enterSafeModeLocked(reason string) {
	if e.safeMode {
		return // Already in safe mode
	}

	e.safeMode = true
	e.safeModeAt = time.Now()

	current, peak, drawdown := e.hwm.Snapshot()

	e.logger.Error("KILL SWITCH ACTIVATED - entering safe mode",
		"reason", reason,
		"equity", current,
		"peak", peak,
		"drawdown", drawdown,
	)
}

// getOrCreateSizer returns the sizer for a symbol, creating if needed.
func (e *Engine) getOrCreateSizer(symbol string) (*PositionSizer, error) {
	if sizer, ok := e.sizers[symbol]; ok {
		return sizer, nil
	}

	sizer, err := NewPositionSizerForSymbol(symbol)
	if err != nil {
		return nil, err
	}

	e.sizers[symbol] = sizer
	return sizer, nil
}

// checkExposureLimits checks if adding a position would exceed limits.
// Uses margin-based exposure (more appropriate for futures) rather than notional.
func (e *Engine) checkExposureLimits(symbol string, contracts int, price decimal.Decimal, spec types.InstrumentSpec) error {
	equity := e.hwm.Current()

	// Calculate new position margin requirement
	// Use intraday margin for intraday trading
	marginPerContract := spec.MarginIntra
	if marginPerContract.IsZero() {
		marginPerContract = spec.MarginInitial
	}
	newMargin := marginPerContract.Mul(decimal.NewFromInt(int64(contracts)))

	// Check per-symbol exposure (margin-based)
	maxSymbolExposure := equity.Mul(e.cfg.MaxExposurePerSymbolPct)

	// Add existing position margin for this symbol
	symbolMargin := newMargin
	if pos, ok := e.positions[symbol]; ok {
		existingMargin := marginPerContract.Mul(decimal.NewFromInt(int64(pos.Contracts)))
		symbolMargin = symbolMargin.Add(existingMargin)
	}

	if symbolMargin.GreaterThan(maxSymbolExposure) {
		return fmt.Errorf("%w: symbol margin %.2f exceeds limit %.2f",
			types.ErrExposureLimitExceeded, symbolMargin.InexactFloat64(), maxSymbolExposure.InexactFloat64())
	}

	// Check total exposure (all positions)
	maxTotalExposure := equity.Mul(e.cfg.MaxTotalExposurePct)
	totalMargin := symbolMargin

	for sym, pos := range e.positions {
		if sym == symbol {
			continue // Already counted above
		}
		symSpec, ok := types.GetInstrumentSpec(sym)
		if !ok {
			continue
		}
		symMarginPerContract := symSpec.MarginIntra
		if symMarginPerContract.IsZero() {
			symMarginPerContract = symSpec.MarginInitial
		}
		posMargin := symMarginPerContract.Mul(decimal.NewFromInt(int64(pos.Contracts)))
		totalMargin = totalMargin.Add(posMargin)
	}

	if totalMargin.GreaterThan(maxTotalExposure) {
		return fmt.Errorf("%w: total margin %.2f exceeds limit %.2f",
			types.ErrExposureLimitExceeded, totalMargin.InexactFloat64(), maxTotalExposure.InexactFloat64())
	}

	return nil
}

// generateClientOrderID creates a unique client order ID for idempotency.
func generateClientOrderID() string {
	return fmt.Sprintf("%s-%s",
		time.Now().Format("20060102-150405"),
		uuid.New().String()[:8],
	)
}
