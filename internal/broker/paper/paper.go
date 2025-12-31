// Package paper provides a simulated broker for paper trading.
package paper

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Config holds paper trading configuration.
type Config struct {
	InitialEquity     decimal.Decimal
	SlippageTicks     int
	CommissionPerSide decimal.Decimal
	FillDelay         time.Duration
}

// DefaultConfig returns default paper trading config.
func DefaultConfig() Config {
	return Config{
		InitialEquity:     decimal.NewFromInt(10000),
		SlippageTicks:     1,
		CommissionPerSide: decimal.NewFromFloat(0.62), // MES typical commission
		FillDelay:         50 * time.Millisecond,
	}
}

// Broker implements broker.Broker for paper trading.
type Broker struct {
	cfg    Config
	logger *slog.Logger

	// State
	state atomic.Int32

	// Account
	accountMu sync.RWMutex
	equity    decimal.Decimal
	cash      decimal.Decimal

	// Positions
	positionsMu sync.RWMutex
	positions   map[string]*broker.Position

	// Orders
	ordersMu   sync.RWMutex
	orders     map[string]*broker.Order
	nextOrderID atomic.Int64

	// Market data simulation
	mdMu          sync.RWMutex
	mdSubscriptions map[string]*mdSubscription
	prices        map[string]decimal.Decimal

	// Shutdown
	done chan struct{}
	wg   sync.WaitGroup
}

type mdSubscription struct {
	symbol string
	ch     chan types.MarketEvent
}

// NewBroker creates a new paper trading broker.
func NewBroker(cfg Config, logger *slog.Logger) *Broker {
	if logger == nil {
		logger = slog.Default()
	}

	b := &Broker{
		cfg:             cfg,
		logger:          logger,
		equity:          cfg.InitialEquity,
		cash:            cfg.InitialEquity,
		positions:       make(map[string]*broker.Position),
		orders:          make(map[string]*broker.Order),
		mdSubscriptions: make(map[string]*mdSubscription),
		prices:          make(map[string]decimal.Decimal),
		done:            make(chan struct{}),
	}

	b.state.Store(int32(broker.StateDisconnected))
	b.nextOrderID.Store(1)

	return b
}

// Connect simulates connecting to broker.
func (b *Broker) Connect(ctx context.Context) error {
	b.state.Store(int32(broker.StateConnected))
	b.logger.Info("paper broker connected",
		"equity", b.cfg.InitialEquity,
	)
	return nil
}

// Disconnect simulates disconnecting from broker.
func (b *Broker) Disconnect() error {
	b.state.Store(int32(broker.StateDisconnected))
	close(b.done)
	b.wg.Wait()
	b.logger.Info("paper broker disconnected")
	return nil
}

// State returns connection state.
func (b *Broker) State() broker.ConnectionState {
	return broker.ConnectionState(b.state.Load())
}

// IsConnected returns true if connected.
func (b *Broker) IsConnected() bool {
	return b.State() == broker.StateConnected
}

// GetAccountSummary returns simulated account summary.
func (b *Broker) GetAccountSummary(ctx context.Context) (*broker.AccountSummary, error) {
	b.accountMu.RLock()
	defer b.accountMu.RUnlock()

	unrealizedPnL := b.calculateUnrealizedPnL()

	return &broker.AccountSummary{
		AccountID:       "PAPER",
		Currency:        "USD",
		NetLiquidation:  b.equity.Add(unrealizedPnL),
		TotalCashValue:  b.cash,
		BuyingPower:     b.cash.Mul(decimal.NewFromInt(4)), // 4x for futures
		AvailableFunds:  b.cash,
		UnrealizedPnL:   unrealizedPnL,
		LastUpdated:     time.Now(),
	}, nil
}

// calculateUnrealizedPnL calculates total unrealized P&L.
func (b *Broker) calculateUnrealizedPnL() decimal.Decimal {
	b.positionsMu.RLock()
	defer b.positionsMu.RUnlock()

	total := decimal.Zero
	for _, pos := range b.positions {
		total = total.Add(pos.UnrealizedPnL)
	}
	return total
}

// SubscribeMarketData subscribes to market data.
func (b *Broker) SubscribeMarketData(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	b.mdMu.Lock()
	defer b.mdMu.Unlock()

	if sub, ok := b.mdSubscriptions[symbol]; ok {
		return sub.ch, nil
	}

	ch := make(chan types.MarketEvent, 100)
	b.mdSubscriptions[symbol] = &mdSubscription{
		symbol: symbol,
		ch:     ch,
	}

	b.logger.Info("subscribed to market data", "symbol", symbol)
	return ch, nil
}

// UnsubscribeMarketData unsubscribes from market data.
func (b *Broker) UnsubscribeMarketData(symbol string) error {
	b.mdMu.Lock()
	defer b.mdMu.Unlock()

	if sub, ok := b.mdSubscriptions[symbol]; ok {
		close(sub.ch)
		delete(b.mdSubscriptions, symbol)
	}

	return nil
}

// SimulateMarketData simulates market data for testing.
func (b *Broker) SimulateMarketData(event types.MarketEvent) {
	b.mdMu.RLock()
	defer b.mdMu.RUnlock()

	// Update price
	b.prices[event.Symbol] = event.Close

	// Update position P&L
	b.updatePositionPnL(event.Symbol, event.Close)

	// Publish to subscribers
	if sub, ok := b.mdSubscriptions[event.Symbol]; ok {
		select {
		case sub.ch <- event:
		default:
		}
	}
}

// updatePositionPnL updates position unrealized P&L.
func (b *Broker) updatePositionPnL(symbol string, price decimal.Decimal) {
	b.positionsMu.Lock()
	defer b.positionsMu.Unlock()

	pos, ok := b.positions[symbol]
	if !ok {
		return
	}

	spec, _ := types.GetInstrumentSpec(symbol)
	tickValue := spec.TickValue

	priceDiff := price.Sub(pos.AvgCost)
	ticksDiff := priceDiff.Div(spec.TickSize)

	pnl := ticksDiff.Mul(tickValue).Mul(decimal.NewFromInt(int64(pos.Contracts)))
	if pos.Side == types.SideShort {
		pnl = pnl.Neg()
	}

	pos.MarketPrice = price
	pos.UnrealizedPnL = pnl
	pos.LastUpdated = time.Now()
}

// PlaceOrder simulates order placement.
func (b *Broker) PlaceOrder(ctx context.Context, intent types.OrderIntent) (*broker.OrderResult, error) {
	if !b.IsConnected() {
		return nil, broker.ErrNotConnected
	}

	orderID := fmt.Sprintf("PAPER-%d", b.nextOrderID.Add(1))

	order := &broker.Order{
		OrderID:       orderID,
		ClientOrderID: intent.ClientOrderID,
		Symbol:        intent.Symbol,
		Side:          intent.Side,
		Quantity:      intent.Contracts,
		OrderType:     broker.OrderTypeMarket,
		Status:        broker.OrderStatusSubmitted,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	b.ordersMu.Lock()
	b.orders[orderID] = order
	b.ordersMu.Unlock()

	b.logger.Info("paper order placed",
		"order_id", orderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"contracts", intent.Contracts,
	)

	// Simulate fill after delay
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.simulateFill(order, intent)
	}()

	return &broker.OrderResult{
		OrderID:       orderID,
		ClientOrderID: intent.ClientOrderID,
		Status:        broker.OrderStatusSubmitted,
		SubmittedAt:   time.Now(),
	}, nil
}

// simulateFill simulates order fill.
func (b *Broker) simulateFill(order *broker.Order, intent types.OrderIntent) {
	select {
	case <-b.done:
		return
	case <-time.After(b.cfg.FillDelay):
	}

	// Get current price
	b.mdMu.RLock()
	price, ok := b.prices[intent.Symbol]
	b.mdMu.RUnlock()

	if !ok {
		// Use entry price if no market data
		price = intent.EntryPrice
	}

	// Apply slippage
	spec, _ := types.GetInstrumentSpec(intent.Symbol)
	slippage := spec.TickSize.Mul(decimal.NewFromInt(int64(b.cfg.SlippageTicks)))

	if intent.Side == types.SideLong {
		price = price.Add(slippage)
	} else {
		price = price.Sub(slippage)
	}

	// Calculate commission
	commission := b.cfg.CommissionPerSide.Mul(decimal.NewFromInt(int64(intent.Contracts)))

	// Update order
	b.ordersMu.Lock()
	order.Status = broker.OrderStatusFilled
	order.FilledQty = intent.Contracts
	order.AvgFillPrice = price
	order.Commission = commission
	order.UpdatedAt = time.Now()
	b.ordersMu.Unlock()

	// Update position
	b.updatePosition(intent.Symbol, intent.Side, intent.Contracts, price)

	// Deduct commission
	b.accountMu.Lock()
	b.cash = b.cash.Sub(commission)
	b.accountMu.Unlock()

	b.logger.Info("paper order filled",
		"order_id", order.OrderID,
		"symbol", intent.Symbol,
		"side", intent.Side,
		"contracts", intent.Contracts,
		"price", price,
		"commission", commission,
	)
}

// updatePosition updates position after fill.
func (b *Broker) updatePosition(symbol string, side types.Side, contracts int, price decimal.Decimal) {
	b.positionsMu.Lock()
	defer b.positionsMu.Unlock()

	pos, exists := b.positions[symbol]

	if !exists {
		// New position
		b.positions[symbol] = &broker.Position{
			Symbol:      symbol,
			Contracts:   contracts,
			Side:        side,
			AvgCost:     price,
			MarketPrice: price,
			LastUpdated: time.Now(),
		}
		return
	}

	// Existing position
	if pos.Side == side {
		// Adding to position
		totalCost := pos.AvgCost.Mul(decimal.NewFromInt(int64(pos.Contracts)))
		newCost := price.Mul(decimal.NewFromInt(int64(contracts)))
		totalContracts := pos.Contracts + contracts

		pos.AvgCost = totalCost.Add(newCost).Div(decimal.NewFromInt(int64(totalContracts)))
		pos.Contracts = totalContracts
	} else {
		// Closing position
		if contracts >= pos.Contracts {
			// Full close or flip
			closedContracts := pos.Contracts
			remainingContracts := contracts - closedContracts

			// Realize P&L
			b.realizePositionPnL(pos, price, closedContracts)

			if remainingContracts > 0 {
				// Flip position
				pos.Side = side
				pos.Contracts = remainingContracts
				pos.AvgCost = price
				pos.UnrealizedPnL = decimal.Zero
			} else {
				// Full close
				delete(b.positions, symbol)
				return
			}
		} else {
			// Partial close
			b.realizePositionPnL(pos, price, contracts)
			pos.Contracts -= contracts
		}
	}

	pos.MarketPrice = price
	pos.LastUpdated = time.Now()
}

// realizePositionPnL realizes P&L from closing contracts.
func (b *Broker) realizePositionPnL(pos *broker.Position, exitPrice decimal.Decimal, contracts int) {
	spec, _ := types.GetInstrumentSpec(pos.Symbol)

	priceDiff := exitPrice.Sub(pos.AvgCost)
	ticksDiff := priceDiff.Div(spec.TickSize)
	pnl := ticksDiff.Mul(spec.TickValue).Mul(decimal.NewFromInt(int64(contracts)))

	if pos.Side == types.SideShort {
		pnl = pnl.Neg()
	}

	b.accountMu.Lock()
	b.cash = b.cash.Add(pnl)
	b.equity = b.equity.Add(pnl)
	b.accountMu.Unlock()

	b.logger.Info("realized P&L",
		"symbol", pos.Symbol,
		"contracts", contracts,
		"entry", pos.AvgCost,
		"exit", exitPrice,
		"pnl", pnl,
	)
}

// CancelOrder cancels an order.
func (b *Broker) CancelOrder(ctx context.Context, orderID string) error {
	b.ordersMu.Lock()
	defer b.ordersMu.Unlock()

	order, ok := b.orders[orderID]
	if !ok {
		return nil
	}

	if order.Status == broker.OrderStatusSubmitted {
		order.Status = broker.OrderStatusCancelled
		order.UpdatedAt = time.Now()
	}

	return nil
}

// GetOpenOrders returns open orders.
func (b *Broker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	b.ordersMu.RLock()
	defer b.ordersMu.RUnlock()

	var orders []broker.Order
	for _, o := range b.orders {
		if o.Status == broker.OrderStatusSubmitted || o.Status == broker.OrderStatusPartial {
			orders = append(orders, *o)
		}
	}

	return orders, nil
}

// GetPositions returns all positions.
func (b *Broker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	b.positionsMu.RLock()
	defer b.positionsMu.RUnlock()

	var positions []broker.Position
	for _, p := range b.positions {
		positions = append(positions, *p)
	}

	return positions, nil
}

// GetPosition returns position for a symbol.
func (b *Broker) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	b.positionsMu.RLock()
	defer b.positionsMu.RUnlock()

	pos, ok := b.positions[symbol]
	if !ok {
		return nil, nil
	}

	return pos, nil
}

// Shutdown shuts down the broker.
func (b *Broker) Shutdown(ctx context.Context) error {
	return b.Disconnect()
}

// GetEquity returns current equity.
func (b *Broker) GetEquity() decimal.Decimal {
	b.accountMu.RLock()
	defer b.accountMu.RUnlock()
	return b.equity.Add(b.calculateUnrealizedPnL())
}

// Ensure Broker implements broker.Broker
var _ broker.Broker = (*Broker)(nil)
