// Package backtest provides backtesting functionality.
package backtest

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/execution"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/strategy"
	"github.com/tathienbao/quant-bot/internal/types"
)

// ProgressUpdate contains info for UI updates
type ProgressUpdate struct {
	Bar         int
	TotalBars   int
	Event       types.MarketEvent
	Equity      decimal.Decimal
	Trades      int
	WinRate     decimal.Decimal
	LastSignal  string
}

// ProgressCallback is called on each bar for UI updates
type ProgressCallback func(update ProgressUpdate)

// Config holds backtest configuration.
type Config struct {
	InitialEquity decimal.Decimal
	StartTime     time.Time
	EndTime       time.Time
}

// Result holds backtest results.
type Result struct {
	StartEquity   decimal.Decimal
	EndEquity     decimal.Decimal
	TotalReturn   decimal.Decimal // As ratio (0.15 = 15%)
	MaxDrawdown   decimal.Decimal // As ratio
	TotalTrades   int
	WinningTrades int
	LosingTrades  int
	WinRate       decimal.Decimal // As ratio
	ProfitFactor  decimal.Decimal // Gross profit / Gross loss
	SharpeRatio   decimal.Decimal
	Trades        []types.Trade
	EquityCurve   []EquityPoint
}

// EquityPoint represents equity at a point in time.
type EquityPoint struct {
	Timestamp time.Time
	Equity    decimal.Decimal
	Drawdown  decimal.Decimal
}

// Runner executes backtests.
type Runner struct {
	cfg        Config
	feed       observer.MarketDataFeed
	calculator *observer.Calculator
	strategy   strategy.Strategy
	riskEngine *risk.Engine
	executor   *execution.SimulatedExecutor

	equityCurve []EquityPoint
	highWater   decimal.Decimal

	// UI callback
	progressCb ProgressCallback
	barCount   int
	totalBars  int
}

// NewRunner creates a new backtest runner.
func NewRunner(
	cfg Config,
	feed observer.MarketDataFeed,
	calculator *observer.Calculator,
	strat strategy.Strategy,
	riskCfg risk.Config,
	execCfg execution.SimulatedConfig,
) *Runner {
	// Use nil logger - risk engine will use slog.Default()
	riskEngine := risk.NewEngine(riskCfg, cfg.InitialEquity, nil)

	executor := execution.NewSimulatedExecutor(execCfg)

	return &Runner{
		cfg:         cfg,
		feed:        feed,
		calculator:  calculator,
		strategy:    strat,
		riskEngine:  riskEngine,
		executor:    executor,
		equityCurve: make([]EquityPoint, 0),
		highWater:   cfg.InitialEquity,
	}
}

// SetProgressCallback sets a callback for UI updates
func (r *Runner) SetProgressCallback(cb ProgressCallback) {
	r.progressCb = cb
}

// SetTotalBars sets the expected total number of bars (for progress display)
func (r *Runner) SetTotalBars(total int) {
	r.totalBars = total
}

// Run executes the backtest.
func (r *Runner) Run(ctx context.Context) (*Result, error) {
	return r.RunSymbol(ctx, "")
}

// RunSymbol executes the backtest for a specific symbol.
func (r *Runner) RunSymbol(ctx context.Context, symbol string) (*Result, error) {
	// Subscribe to market data
	eventCh, err := r.feed.Subscribe(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("subscribe to feed: %w", err)
	}

	currentEquity := r.cfg.InitialEquity

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case event, ok := <-eventCh:
			if !ok {
				// Feed closed, backtest complete
				return r.calculateResults(), nil
			}

			// Apply time filters
			if !r.cfg.StartTime.IsZero() && event.Timestamp.Before(r.cfg.StartTime) {
				continue
			}
			if !r.cfg.EndTime.IsZero() && event.Timestamp.After(r.cfg.EndTime) {
				return r.calculateResults(), nil
			}

			r.barCount++

			// Calculate indicators
			if r.calculator != nil {
				event = r.calculator.OnBar(event)
			}

			// Update executor with market data (check stops/TPs)
			fills := r.executor.UpdateMarket(event)
			for _, fill := range fills {
				currentEquity = r.updateEquity(currentEquity, fill, event.Timestamp)
			}

			// Generate signals from strategy
			signals := r.strategy.OnMarketEvent(ctx, event)
			var lastSignal string

			// Process each signal through risk engine
			for _, signal := range signals {
				orderIntent, err := r.riskEngine.ValidateAndSize(ctx, signal, event)
				if err != nil {
					// Signal rejected by risk engine (expected behavior)
					continue
				}

				// Execute order
				result, err := r.executor.PlaceOrder(ctx, *orderIntent)
				if err != nil {
					continue
				}

				// Update equity if order resulted in a trade close
				if result.Status == types.OrderStatusFilled {
					// For opening orders, no immediate PnL
					// PnL realized on close via UpdateMarket
					lastSignal = signal.Direction.String()
				}
			}

			// Record equity point
			r.recordEquity(event.Timestamp, currentEquity)

			// Call progress callback for UI
			if r.progressCb != nil {
				trades := r.executor.GetTrades()
				winRate := decimal.Zero
				winCount := 0
				for _, t := range trades {
					if t.NetPL.IsPositive() {
						winCount++
					}
				}
				if len(trades) > 0 {
					winRate = decimal.NewFromInt(int64(winCount)).Div(decimal.NewFromInt(int64(len(trades)))).Mul(decimal.NewFromInt(100))
				}

				r.progressCb(ProgressUpdate{
					Bar:        r.barCount,
					TotalBars:  r.totalBars,
					Event:      event,
					Equity:     currentEquity,
					Trades:     len(trades),
					WinRate:    winRate,
					LastSignal: lastSignal,
				})
			}
		}
	}
}

// updateEquity updates equity after a fill.
func (r *Runner) updateEquity(currentEquity decimal.Decimal, fill types.OrderResult, timestamp time.Time) decimal.Decimal {
	// Find the trade that matches this fill
	trades := r.executor.GetTrades()
	if len(trades) == 0 {
		return currentEquity
	}

	lastTrade := trades[len(trades)-1]
	newEquity := currentEquity.Add(lastTrade.NetPL)

	// Update risk engine
	r.riskEngine.UpdateEquity(newEquity)

	// Update high water mark
	if newEquity.GreaterThan(r.highWater) {
		r.highWater = newEquity
	}

	return newEquity
}

// recordEquity records an equity point.
func (r *Runner) recordEquity(timestamp time.Time, equity decimal.Decimal) {
	var drawdown decimal.Decimal
	if r.highWater.IsPositive() {
		drawdown = r.highWater.Sub(equity).Div(r.highWater)
	}

	r.equityCurve = append(r.equityCurve, EquityPoint{
		Timestamp: timestamp,
		Equity:    equity,
		Drawdown:  drawdown,
	})
}

// calculateResults computes final backtest results.
func (r *Runner) calculateResults() *Result {
	trades := r.executor.GetTrades()

	var (
		endEquity     = r.cfg.InitialEquity
		maxDrawdown   = decimal.Zero
		winningTrades = 0
		losingTrades  = 0
		grossProfit   = decimal.Zero
		grossLoss     = decimal.Zero
	)

	// Calculate end equity from trades
	for _, trade := range trades {
		endEquity = endEquity.Add(trade.NetPL)
		if trade.NetPL.IsPositive() {
			winningTrades++
			grossProfit = grossProfit.Add(trade.NetPL)
		} else if trade.NetPL.IsNegative() {
			losingTrades++
			grossLoss = grossLoss.Add(trade.NetPL.Abs())
		}
	}

	// Calculate max drawdown from equity curve
	hwm := r.cfg.InitialEquity
	for _, point := range r.equityCurve {
		if point.Equity.GreaterThan(hwm) {
			hwm = point.Equity
		}
		dd := hwm.Sub(point.Equity).Div(hwm)
		if dd.GreaterThan(maxDrawdown) {
			maxDrawdown = dd
		}
	}

	// Calculate metrics
	totalReturn := decimal.Zero
	if r.cfg.InitialEquity.IsPositive() {
		totalReturn = endEquity.Sub(r.cfg.InitialEquity).Div(r.cfg.InitialEquity)
	}

	winRate := decimal.Zero
	if len(trades) > 0 {
		winRate = decimal.NewFromInt(int64(winningTrades)).Div(decimal.NewFromInt(int64(len(trades))))
	}

	profitFactor := decimal.Zero
	if grossLoss.IsPositive() {
		profitFactor = grossProfit.Div(grossLoss)
	}

	return &Result{
		StartEquity:   r.cfg.InitialEquity,
		EndEquity:     endEquity,
		TotalReturn:   totalReturn,
		MaxDrawdown:   maxDrawdown,
		TotalTrades:   len(trades),
		WinningTrades: winningTrades,
		LosingTrades:  losingTrades,
		WinRate:       winRate,
		ProfitFactor:  profitFactor,
		Trades:        trades,
		EquityCurve:   r.equityCurve,
	}
}

// Reset resets the runner for a new backtest.
func (r *Runner) Reset() {
	r.executor.Reset()
	r.strategy.Reset()
	r.equityCurve = make([]EquityPoint, 0)
	r.highWater = r.cfg.InitialEquity
	r.barCount = 0

	// Reset risk engine to initial equity
	r.riskEngine.UpdateEquity(r.cfg.InitialEquity)
}
