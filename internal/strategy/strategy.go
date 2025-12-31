// Package strategy implements trading strategies.
package strategy

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Strategy defines the interface for trading strategies.
// Strategies receive market events and generate trading signals.
// They should NOT handle position sizing or risk management.
type Strategy interface {
	// OnMarketEvent processes a market event and returns any signals.
	// Returns nil or empty slice if no signal is generated.
	OnMarketEvent(ctx context.Context, event types.MarketEvent) []types.Signal

	// Name returns the strategy identifier.
	Name() string

	// Reset clears all strategy state.
	Reset()
}

// SignalBuilder helps construct signals with consistent defaults.
type SignalBuilder struct {
	signal types.Signal
}

// NewSignalBuilder creates a new signal builder.
func NewSignalBuilder(strategyName string, event types.MarketEvent) *SignalBuilder {
	return &SignalBuilder{
		signal: types.Signal{
			ID:           uuid.New().String(),
			Timestamp:    event.Timestamp,
			Symbol:       event.Symbol,
			StrategyName: strategyName,
		},
	}
}

// Long sets the signal direction to long.
func (b *SignalBuilder) Long() *SignalBuilder {
	b.signal.Direction = types.SideLong
	return b
}

// Short sets the signal direction to short.
func (b *SignalBuilder) Short() *SignalBuilder {
	b.signal.Direction = types.SideShort
	return b
}

// Flat sets the signal direction to flat (exit).
func (b *SignalBuilder) Flat() *SignalBuilder {
	b.signal.Direction = types.SideFlat
	return b
}

// WithStopTicks sets the stop distance in ticks.
func (b *SignalBuilder) WithStopTicks(ticks int) *SignalBuilder {
	b.signal.StopTicks = ticks
	return b
}

// WithStrength sets the signal strength (0-1).
func (b *SignalBuilder) WithStrength(strength decimal.Decimal) *SignalBuilder {
	b.signal.Strength = strength
	return b
}

// WithReason sets the signal reason.
func (b *SignalBuilder) WithReason(reason string) *SignalBuilder {
	b.signal.Reason = reason
	return b
}

// WithATRStop sets the stop distance based on ATR.
func (b *SignalBuilder) WithATRStop(atr decimal.Decimal, multiplier decimal.Decimal, tickSize decimal.Decimal) *SignalBuilder {
	if atr.IsZero() || tickSize.IsZero() {
		return b
	}
	stopDistance := atr.Mul(multiplier)
	b.signal.StopTicks = int(stopDistance.Div(tickSize).Ceil().IntPart())
	return b
}

// Build returns the constructed signal.
func (b *SignalBuilder) Build() types.Signal {
	return b.signal
}

// MultiStrategy combines multiple strategies.
type MultiStrategy struct {
	strategies []Strategy
	name       string
}

// NewMultiStrategy creates a strategy that runs multiple sub-strategies.
func NewMultiStrategy(name string, strategies ...Strategy) *MultiStrategy {
	return &MultiStrategy{
		strategies: strategies,
		name:       name,
	}
}

// OnMarketEvent processes event through all strategies.
func (m *MultiStrategy) OnMarketEvent(ctx context.Context, event types.MarketEvent) []types.Signal {
	var allSignals []types.Signal

	for _, s := range m.strategies {
		select {
		case <-ctx.Done():
			return allSignals
		default:
			signals := s.OnMarketEvent(ctx, event)
			allSignals = append(allSignals, signals...)
		}
	}

	return allSignals
}

// Name returns the multi-strategy name.
func (m *MultiStrategy) Name() string {
	return m.name
}

// Reset resets all sub-strategies.
func (m *MultiStrategy) Reset() {
	for _, s := range m.strategies {
		s.Reset()
	}
}
