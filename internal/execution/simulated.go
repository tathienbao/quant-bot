package execution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// SimulatedConfig holds configuration for the simulated executor.
type SimulatedConfig struct {
	SlippageTicks    int             // Fixed slippage in ticks
	CommissionPerSide decimal.Decimal // Commission per contract per side
	FillDelayMs      int             // Simulated fill delay
}

// DefaultSimulatedConfig returns sensible defaults.
func DefaultSimulatedConfig() SimulatedConfig {
	return SimulatedConfig{
		SlippageTicks:    1,
		CommissionPerSide: decimal.RequireFromString("0.62"), // Typical futures commission
		FillDelayMs:      0,
	}
}

// SimulatedExecutor simulates order execution for backtesting.
type SimulatedExecutor struct {
	cfg SimulatedConfig

	mu           sync.RWMutex
	positions    map[string]*types.Position // symbol -> position
	openOrders   map[string]*types.OrderIntent // clientOrderID -> order
	usedOrderIDs map[string]bool // Track all used client order IDs for idempotency
	orderHistory []types.OrderResult
	trades       []types.Trade

	fillHandler FillHandler
	currentTime time.Time
	currentPrice map[string]decimal.Decimal // symbol -> current price
}

// NewSimulatedExecutor creates a new simulated executor.
func NewSimulatedExecutor(cfg SimulatedConfig) *SimulatedExecutor {
	return &SimulatedExecutor{
		cfg:          cfg,
		positions:    make(map[string]*types.Position),
		openOrders:   make(map[string]*types.OrderIntent),
		usedOrderIDs: make(map[string]bool),
		orderHistory: make([]types.OrderResult, 0),
		trades:       make([]types.Trade, 0),
		currentPrice: make(map[string]decimal.Decimal),
	}
}

// SetFillHandler sets the callback for fill events.
func (s *SimulatedExecutor) SetFillHandler(handler FillHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fillHandler = handler
}

// UpdateMarket updates the current market price and time.
// This is called by the backtest runner for each market event.
func (s *SimulatedExecutor) UpdateMarket(event types.MarketEvent) []types.OrderResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentTime = event.Timestamp
	s.currentPrice[event.Symbol] = event.Close

	// Check for stop loss / take profit fills
	var fills []types.OrderResult
	if pos, ok := s.positions[event.Symbol]; ok && pos.Contracts > 0 {
		fills = append(fills, s.checkExits(event, pos)...)
	}

	return fills
}

// checkExits checks if stop loss or take profit is hit.
func (s *SimulatedExecutor) checkExits(event types.MarketEvent, pos *types.Position) []types.OrderResult {
	var fills []types.OrderResult

	if pos.Side == types.SideLong {
		// Long position: stop below entry, TP above
		if !pos.StopLoss.IsZero() && event.Low.LessThanOrEqual(pos.StopLoss) {
			fill := s.closePosition(pos, pos.StopLoss, "stop_loss")
			fills = append(fills, fill)
		} else if !pos.TakeProfit.IsZero() && event.High.GreaterThanOrEqual(pos.TakeProfit) {
			fill := s.closePosition(pos, pos.TakeProfit, "take_profit")
			fills = append(fills, fill)
		}
	} else if pos.Side == types.SideShort {
		// Short position: stop above entry, TP below
		if !pos.StopLoss.IsZero() && event.High.GreaterThanOrEqual(pos.StopLoss) {
			fill := s.closePosition(pos, pos.StopLoss, "stop_loss")
			fills = append(fills, fill)
		} else if !pos.TakeProfit.IsZero() && event.Low.LessThanOrEqual(pos.TakeProfit) {
			fill := s.closePosition(pos, pos.TakeProfit, "take_profit")
			fills = append(fills, fill)
		}
	}

	return fills
}

// closePosition closes a position at the given price.
func (s *SimulatedExecutor) closePosition(pos *types.Position, exitPrice decimal.Decimal, reason string) types.OrderResult {
	spec, _ := types.GetInstrumentSpec(pos.Symbol)

	// Apply slippage (against us)
	slippageAmount := spec.TickSize.Mul(decimal.NewFromInt(int64(s.cfg.SlippageTicks)))
	if pos.Side == types.SideLong {
		exitPrice = exitPrice.Sub(slippageAmount) // Sell lower
	} else {
		exitPrice = exitPrice.Add(slippageAmount) // Buy higher
	}

	// Calculate PnL
	var grossPL decimal.Decimal
	if pos.Side == types.SideLong {
		grossPL = exitPrice.Sub(pos.EntryPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
	} else {
		grossPL = pos.EntryPrice.Sub(exitPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
	}

	commission := s.cfg.CommissionPerSide.Mul(decimal.NewFromInt(int64(pos.Contracts)))
	netPL := grossPL.Sub(commission)

	// Create trade record
	trade := types.Trade{
		ID:           uuid.New().String(),
		Symbol:       pos.Symbol,
		Side:         pos.Side,
		Contracts:    pos.Contracts,
		EntryPrice:   pos.EntryPrice,
		ExitPrice:    exitPrice,
		EntryTime:    pos.EntryTime,
		ExitTime:     s.currentTime,
		GrossPL:      grossPL,
		Commission:   commission,
		NetPL:        netPL,
	}
	s.trades = append(s.trades, trade)

	// Clear position
	delete(s.positions, pos.Symbol)

	result := types.OrderResult{
		OrderID:       uuid.New().String(),
		ClientOrderID: reason + "-" + pos.ID,
		Status:        types.OrderStatusFilled,
		FilledQty:     pos.Contracts,
		AvgFillPrice:  exitPrice,
		Commission:    commission,
		Slippage:      slippageAmount,
		FilledAt:      s.currentTime,
	}

	s.orderHistory = append(s.orderHistory, result)

	// Notify fill handler
	if s.fillHandler != nil {
		go s.fillHandler(context.Background(), result)
	}

	return result
}

// PlaceOrder submits an order for execution.
func (s *SimulatedExecutor) PlaceOrder(ctx context.Context, order types.OrderIntent) (*types.OrderResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate (idempotency)
	if s.usedOrderIDs[order.ClientOrderID] {
		return nil, types.ErrDuplicateOrder
	}
	s.usedOrderIDs[order.ClientOrderID] = true

	spec, ok := types.GetInstrumentSpec(order.Symbol)
	if !ok {
		return nil, fmt.Errorf("unknown symbol: %s", order.Symbol)
	}

	// Get current price
	currentPrice, ok := s.currentPrice[order.Symbol]
	if !ok {
		return nil, fmt.Errorf("no market data for symbol: %s", order.Symbol)
	}

	// Calculate fill price with slippage
	slippageAmount := spec.TickSize.Mul(decimal.NewFromInt(int64(s.cfg.SlippageTicks)))
	var fillPrice decimal.Decimal
	if order.Side == types.SideLong {
		fillPrice = currentPrice.Add(slippageAmount) // Buy higher
	} else {
		fillPrice = currentPrice.Sub(slippageAmount) // Sell lower
	}

	// Calculate commission
	commission := s.cfg.CommissionPerSide.Mul(decimal.NewFromInt(int64(order.Contracts)))

	// Check if we're closing an existing position
	existingPos, hasPosition := s.positions[order.Symbol]
	if hasPosition && existingPos.Side == order.Side.Opposite() {
		// Closing position
		return s.handleCloseOrder(order, existingPos, fillPrice, commission, slippageAmount)
	}

	// Opening new position
	return s.handleOpenOrder(order, fillPrice, commission, slippageAmount)
}

// handleOpenOrder handles opening a new position.
func (s *SimulatedExecutor) handleOpenOrder(order types.OrderIntent, fillPrice, commission, slippage decimal.Decimal) (*types.OrderResult, error) {
	// Create position
	pos := &types.Position{
		ID:         uuid.New().String(),
		Symbol:     order.Symbol,
		Side:       order.Side,
		Contracts:  order.Contracts,
		EntryPrice: fillPrice,
		EntryTime:  s.currentTime,
		StopLoss:   order.StopLoss,
		TakeProfit: order.TakeProfit,
	}
	s.positions[order.Symbol] = pos

	result := &types.OrderResult{
		OrderID:       uuid.New().String(),
		ClientOrderID: order.ClientOrderID,
		Status:        types.OrderStatusFilled,
		FilledQty:     order.Contracts,
		AvgFillPrice:  fillPrice,
		Commission:    commission,
		Slippage:      slippage,
		FilledAt:      s.currentTime,
	}

	s.orderHistory = append(s.orderHistory, *result)

	// Notify fill handler
	if s.fillHandler != nil {
		go s.fillHandler(context.Background(), *result)
	}

	return result, nil
}

// handleCloseOrder handles closing an existing position.
func (s *SimulatedExecutor) handleCloseOrder(order types.OrderIntent, pos *types.Position, fillPrice, commission, slippage decimal.Decimal) (*types.OrderResult, error) {
	spec, _ := types.GetInstrumentSpec(order.Symbol)

	// Calculate PnL
	var grossPL decimal.Decimal
	if pos.Side == types.SideLong {
		grossPL = fillPrice.Sub(pos.EntryPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
	} else {
		grossPL = pos.EntryPrice.Sub(fillPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
	}

	netPL := grossPL.Sub(commission)

	// Create trade record
	trade := types.Trade{
		ID:         uuid.New().String(),
		Symbol:     pos.Symbol,
		Side:       pos.Side,
		Contracts:  pos.Contracts,
		EntryPrice: pos.EntryPrice,
		ExitPrice:  fillPrice,
		EntryTime:  pos.EntryTime,
		ExitTime:   s.currentTime,
		GrossPL:    grossPL,
		Commission: commission,
		NetPL:      netPL,
		SignalID:   order.SignalID,
	}
	s.trades = append(s.trades, trade)

	// Clear position
	delete(s.positions, order.Symbol)

	result := &types.OrderResult{
		OrderID:       uuid.New().String(),
		ClientOrderID: order.ClientOrderID,
		Status:        types.OrderStatusFilled,
		FilledQty:     order.Contracts,
		AvgFillPrice:  fillPrice,
		Commission:    commission,
		Slippage:      slippage,
		FilledAt:      s.currentTime,
	}

	s.orderHistory = append(s.orderHistory, *result)

	// Notify fill handler
	if s.fillHandler != nil {
		go s.fillHandler(context.Background(), *result)
	}

	return result, nil
}

// CancelOrder cancels a pending order.
func (s *SimulatedExecutor) CancelOrder(ctx context.Context, clientOrderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.openOrders[clientOrderID]; !exists {
		return fmt.Errorf("order not found: %s", clientOrderID)
	}

	delete(s.openOrders, clientOrderID)
	return nil
}

// GetPosition returns the current position for a symbol.
func (s *SimulatedExecutor) GetPosition(ctx context.Context, symbol string) (*types.Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pos, ok := s.positions[symbol]
	if !ok {
		return nil, nil // No position is not an error
	}

	// Update unrealized PL
	if currentPrice, ok := s.currentPrice[symbol]; ok {
		spec, _ := types.GetInstrumentSpec(symbol)
		posCopy := *pos
		if pos.Side == types.SideLong {
			posCopy.UnrealizedPL = currentPrice.Sub(pos.EntryPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
		} else {
			posCopy.UnrealizedPL = pos.EntryPrice.Sub(currentPrice).Mul(spec.PointValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
		}
		return &posCopy, nil
	}

	return pos, nil
}

// GetOpenOrders returns all open orders.
func (s *SimulatedExecutor) GetOpenOrders(ctx context.Context) ([]types.OrderIntent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orders := make([]types.OrderIntent, 0, len(s.openOrders))
	for _, o := range s.openOrders {
		orders = append(orders, *o)
	}
	return orders, nil
}

// Shutdown gracefully shuts down the executor.
func (s *SimulatedExecutor) Shutdown(ctx context.Context) error {
	return nil
}

// GetTrades returns all completed trades.
func (s *SimulatedExecutor) GetTrades() []types.Trade {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trades := make([]types.Trade, len(s.trades))
	copy(trades, s.trades)
	return trades
}

// GetPositions returns all open positions.
func (s *SimulatedExecutor) GetPositions() map[string]*types.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()

	positions := make(map[string]*types.Position)
	for k, v := range s.positions {
		posCopy := *v
		positions[k] = &posCopy
	}
	return positions
}

// Reset clears all state.
func (s *SimulatedExecutor) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.positions = make(map[string]*types.Position)
	s.openOrders = make(map[string]*types.OrderIntent)
	s.usedOrderIDs = make(map[string]bool)
	s.orderHistory = make([]types.OrderResult, 0)
	s.trades = make([]types.Trade, 0)
	s.currentPrice = make(map[string]decimal.Decimal)
}
