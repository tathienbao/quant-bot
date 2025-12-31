// Package observer handles market data feeds and indicator calculations.
package observer

import (
	"context"

	"github.com/tathienbao/quant-bot/internal/types"
)

// MarketDataFeed defines the interface for market data sources.
// Implementations can be live feeds or backtest data.
type MarketDataFeed interface {
	// Subscribe starts receiving market events for a symbol.
	// Returns a channel that will receive MarketEvent updates.
	// The channel is closed when the context is cancelled or feed ends.
	Subscribe(ctx context.Context, symbol string) (<-chan types.MarketEvent, error)

	// Close shuts down the feed and releases resources.
	Close() error

	// Name returns the feed identifier (e.g., "backtest", "live-ib").
	Name() string
}

// IndicatorCalculator calculates technical indicators on market data.
type IndicatorCalculator interface {
	// OnBar processes a new bar and updates indicators.
	// Returns the updated MarketEvent with calculated indicators.
	OnBar(event types.MarketEvent) types.MarketEvent

	// Reset clears all indicator state.
	Reset()
}

// Observer combines a data feed with indicator calculations.
type Observer struct {
	feed       MarketDataFeed
	calculator IndicatorCalculator
}

// NewObserver creates a new observer with the given feed and calculator.
func NewObserver(feed MarketDataFeed, calculator IndicatorCalculator) *Observer {
	return &Observer{
		feed:       feed,
		calculator: calculator,
	}
}

// Subscribe starts observing market data for a symbol.
// Returns enriched MarketEvents with calculated indicators.
func (o *Observer) Subscribe(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	rawEvents, err := o.feed.Subscribe(ctx, symbol)
	if err != nil {
		return nil, err
	}

	enrichedEvents := make(chan types.MarketEvent, 100)

	go func() {
		defer close(enrichedEvents)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-rawEvents:
				if !ok {
					return
				}
				// Enrich with indicators
				enriched := o.calculator.OnBar(event)
				select {
				case enrichedEvents <- enriched:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return enrichedEvents, nil
}

// Close shuts down the observer.
func (o *Observer) Close() error {
	return o.feed.Close()
}

// Reset resets the indicator calculator state.
func (o *Observer) Reset() {
	o.calculator.Reset()
}
