// Package engine provides the main trading engine.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/alerting"
	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/metrics"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/strategy"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Config holds engine configuration.
type Config struct {
	Symbol           string
	Timeframe        time.Duration
	EquityUpdateInterval time.Duration
}

// DefaultConfig returns default engine config.
func DefaultConfig() Config {
	return Config{
		Symbol:               "MES",
		Timeframe:            5 * time.Minute,
		EquityUpdateInterval: 1 * time.Minute,
	}
}

// Engine coordinates all trading components.
type Engine struct {
	cfg        Config
	logger     *slog.Logger
	broker     broker.Broker
	riskEngine *risk.Engine
	strategy   strategy.Strategy
	calculator *observer.Calculator
	alerter    alerting.Alerter
	recorder   *metrics.Recorder

	// State
	mu        sync.RWMutex
	running   bool
	lastEvent types.MarketEvent

	// Channels
	done chan struct{}
	wg   sync.WaitGroup
}

// NewEngine creates a new trading engine.
func NewEngine(
	cfg Config,
	brk broker.Broker,
	riskEngine *risk.Engine,
	strat strategy.Strategy,
	calculator *observer.Calculator,
	alerter alerting.Alerter,
	logger *slog.Logger,
) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	return &Engine{
		cfg:        cfg,
		logger:     logger,
		broker:     brk,
		riskEngine: riskEngine,
		strategy:   strat,
		calculator: calculator,
		alerter:    alerter,
		recorder:   metrics.NewRecorder(),
		done:       make(chan struct{}),
	}
}

// Start starts the trading engine.
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("engine already running")
	}
	e.running = true
	e.mu.Unlock()

	e.logger.Info("starting trading engine",
		"symbol", e.cfg.Symbol,
		"strategy", e.strategy.Name(),
	)

	// Subscribe to market data
	marketDataCh, err := e.broker.SubscribeMarketData(ctx, e.cfg.Symbol)
	if err != nil {
		return fmt.Errorf("subscribe market data: %w", err)
	}

	// Start main trading loop
	e.wg.Add(1)
	go e.tradingLoop(ctx, marketDataCh)

	// Start equity update loop
	e.wg.Add(1)
	go e.equityUpdateLoop(ctx)

	// Send start alert
	if e.alerter != nil {
		if err := e.alerter.Alert(ctx, alerting.SeverityInfo, "Trading engine started",
			"symbol", e.cfg.Symbol,
			"strategy", e.strategy.Name(),
		); err != nil {
			e.logger.Warn("failed to send start alert", "err", err)
		}
	}

	return nil
}

// tradingLoop is the main trading loop.
func (e *Engine) tradingLoop(ctx context.Context, marketDataCh <-chan types.MarketEvent) {
	defer e.wg.Done()

	e.logger.Info("trading loop started")

	for {
		select {
		case <-ctx.Done():
			e.logger.Info("trading loop stopped: context cancelled")
			return
		case <-e.done:
			e.logger.Info("trading loop stopped: shutdown requested")
			return
		case event, ok := <-marketDataCh:
			if !ok {
				e.logger.Warn("market data channel closed")
				return
			}

			if err := e.processMarketEvent(ctx, event); err != nil {
				e.logger.Error("failed to process market event", "err", err)
				e.recorder.RecordError("process_event")
			}
		}
	}
}

// processMarketEvent processes a single market event.
func (e *Engine) processMarketEvent(ctx context.Context, event types.MarketEvent) error {
	timer := metrics.NewTimer()

	e.mu.Lock()
	e.lastEvent = event
	e.mu.Unlock()

	// Update calculator
	e.calculator.OnBar(event)

	// Get calculated values
	calcEvent := types.MarketEvent{
		Timestamp: event.Timestamp,
		Symbol:    event.Symbol,
		Open:      event.Open,
		High:      event.High,
		Low:       event.Low,
		Close:     event.Close,
		Volume:    event.Volume,
		ATR:       e.calculator.CurrentATR(),
	}

	// Record heartbeat
	e.recorder.RecordHeartbeat()

	// Generate signals
	signals := e.strategy.OnMarketEvent(ctx, calcEvent)

	timer.ObserveStrategy(e.strategy.Name())

	// Process signals
	for _, signal := range signals {
		e.recorder.RecordSignal(e.strategy.Name(), signal.Direction.String())

		if err := e.processSignal(ctx, signal, calcEvent); err != nil {
			e.logger.Warn("signal rejected",
				"signal_id", signal.ID,
				"reason", err,
			)
		}
	}

	return nil
}

// processSignal processes a trading signal.
func (e *Engine) processSignal(ctx context.Context, signal types.Signal, event types.MarketEvent) error {
	// Check if in safe mode
	if e.riskEngine.IsInSafeMode() {
		e.recorder.RecordSignalRejected("safe_mode")
		return types.ErrKillSwitchActive
	}

	// Validate and size with risk engine
	orderIntent, err := e.riskEngine.ValidateAndSize(ctx, signal, event)
	if err != nil {
		e.recorder.RecordSignalRejected(err.Error())
		return err
	}

	// Place order
	timer := metrics.NewTimer()
	result, err := e.broker.PlaceOrder(ctx, *orderIntent)
	timer.ObserveOrder()

	if err != nil {
		e.recorder.RecordOrder(signal.Symbol, signal.Direction.String(), "rejected")

		// Alert on order rejection
		if e.alerter != nil {
			if alertErr := e.alerter.Alert(ctx, alerting.SeverityWarning, "Order rejected",
				"symbol", signal.Symbol,
				"side", signal.Direction,
				"error", err.Error(),
			); alertErr != nil {
				e.logger.Warn("failed to send order rejection alert", "err", alertErr)
			}
		}

		return fmt.Errorf("place order: %w", err)
	}

	e.recorder.RecordOrder(signal.Symbol, signal.Direction.String(), "submitted")

	e.logger.Info("order placed",
		"order_id", result.OrderID,
		"client_order_id", result.ClientOrderID,
		"symbol", orderIntent.Symbol,
		"side", orderIntent.Side,
		"contracts", orderIntent.Contracts,
		"entry", orderIntent.EntryPrice,
		"stop", orderIntent.StopLoss,
		"target", orderIntent.TakeProfit,
	)

	// Alert on order placement
	if e.alerter != nil {
		if err := e.alerter.Alert(ctx, alerting.SeverityInfo, "Order placed",
			"order_id", result.OrderID,
			"symbol", orderIntent.Symbol,
			"side", orderIntent.Side.String(),
			"contracts", orderIntent.Contracts,
			"entry", orderIntent.EntryPrice.StringFixed(2),
		); err != nil {
			e.logger.Warn("failed to send order placement alert", "err", err)
		}
	}

	return nil
}

// equityUpdateLoop periodically updates equity metrics.
func (e *Engine) equityUpdateLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.cfg.EquityUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.done:
			return
		case <-ticker.C:
			e.updateEquity(ctx)
		}
	}
}

// updateEquity updates equity from broker.
func (e *Engine) updateEquity(ctx context.Context) {
	summary, err := e.broker.GetAccountSummary(ctx)
	if err != nil {
		e.logger.Warn("failed to get account summary", "err", err)
		return
	}

	// Update risk engine
	e.riskEngine.UpdateEquity(summary.NetLiquidation)

	// Update metrics
	snapshot := e.riskEngine.GetSnapshot()
	e.recorder.RecordEquity(snapshot.Equity, snapshot.HighWaterMark, snapshot.Drawdown)
	e.recorder.RecordSafeMode(e.riskEngine.IsInSafeMode())

	// Check for kill switch activation
	if e.riskEngine.IsInSafeMode() {
		e.handleKillSwitch(ctx)
	}
}

// handleKillSwitch handles kill switch activation.
func (e *Engine) handleKillSwitch(ctx context.Context) {
	e.logger.Error("KILL SWITCH ACTIVATED")

	// Alert
	if e.alerter != nil {
		snapshot := e.riskEngine.GetSnapshot()
		if err := e.alerter.Alert(ctx, alerting.SeverityCritical, "KILL SWITCH ACTIVATED",
			"equity", snapshot.Equity.StringFixed(2),
			"high_water", snapshot.HighWaterMark.StringFixed(2),
			"drawdown", snapshot.Drawdown.Mul(decimal.NewFromInt(100)).StringFixed(2)+"%",
		); err != nil {
			e.logger.Error("failed to send kill switch alert", "err", err)
		}
	}

	// Cancel all open orders
	e.cancelAllOrders(ctx)
}

// cancelAllOrders cancels all open orders.
func (e *Engine) cancelAllOrders(ctx context.Context) {
	orders, err := e.broker.GetOpenOrders(ctx)
	if err != nil {
		e.logger.Error("failed to get open orders", "err", err)
		return
	}

	for _, order := range orders {
		if err := e.broker.CancelOrder(ctx, order.OrderID); err != nil {
			e.logger.Error("failed to cancel order",
				"order_id", order.OrderID,
				"err", err,
			)
		}
	}
}

// Stop stops the trading engine.
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	e.mu.Unlock()

	e.logger.Info("stopping trading engine")

	close(e.done)
	e.wg.Wait()

	// Unsubscribe from market data
	if err := e.broker.UnsubscribeMarketData(e.cfg.Symbol); err != nil {
		e.logger.Warn("failed to unsubscribe market data", "err", err)
	}

	// Alert
	if e.alerter != nil {
		if err := e.alerter.Alert(ctx, alerting.SeverityInfo, "Trading engine stopped"); err != nil {
			e.logger.Warn("failed to send stop alert", "err", err)
		}
	}

	e.logger.Info("trading engine stopped")
	return nil
}

// IsRunning returns true if engine is running.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetLastEvent returns the last market event.
func (e *Engine) GetLastEvent() types.MarketEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastEvent
}
