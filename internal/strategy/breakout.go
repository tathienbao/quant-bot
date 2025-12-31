package strategy

/*
╔══════════════════════════════════════════════════════════════════════════════╗
║                         BREAKOUT STRATEGY                                     ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Backtest Results (MES, $100k equity, 1% risk/trade)                         ║
╠═══════════════════╦═══════════════════════╦══════════════════════════════════╣
║  Data             ║  Daily (1 năm)        ║  M5 (2.5 tháng)                  ║
╠═══════════════════╬═══════════════════════╬══════════════════════════════════╣
║  Return           ║  -11.59%              ║  -20.05%                         ║
║  Max Drawdown     ║  11.59%               ║  20.05%                          ║
║  Total Trades     ║  16                   ║  18                              ║
║  Win Rate         ║  0%                   ║  0%                              ║
║  Profit Factor    ║  0.00                 ║  0.00                            ║
╠═══════════════════╩═══════════════════════╩══════════════════════════════════╣
║  ⚠️  KHÔNG KHUYẾN NGHỊ - Strategy này thua lỗ trên cả 2 timeframe            ║
╚══════════════════════════════════════════════════════════════════════════════╝
*/

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// BreakoutConfig holds configuration for the breakout strategy.
type BreakoutConfig struct {
	LookbackBars    int             // Number of bars to look back for high/low
	ATRMultiplier   decimal.Decimal // ATR multiplier for stop loss
	MinATR          decimal.Decimal // Minimum ATR to generate signal
	BreakoutBuffer  decimal.Decimal // Buffer above/below range (as ratio, e.g., 0.001 = 0.1%)
}

// DefaultBreakoutConfig returns sensible defaults.
func DefaultBreakoutConfig() BreakoutConfig {
	return BreakoutConfig{
		LookbackBars:   20,
		ATRMultiplier:  decimal.RequireFromString("2.0"),
		MinATR:         decimal.Zero,
		BreakoutBuffer: decimal.RequireFromString("0.0005"), // 0.05%
	}
}

// Breakout implements a simple range breakout strategy.
// Generates LONG when price breaks above the highest high of N bars.
// Generates SHORT when price breaks below the lowest low of N bars.
type Breakout struct {
	cfg BreakoutConfig

	highs           []decimal.Decimal
	lows            []decimal.Decimal
	signalledLong   bool // Already signalled long for current range
	signalledShort  bool // Already signalled short for current range
	lastRangeHigh   decimal.Decimal
	lastRangeLow    decimal.Decimal
	ready           bool
}

// NewBreakout creates a new breakout strategy.
func NewBreakout(cfg BreakoutConfig) *Breakout {
	return &Breakout{
		cfg:   cfg,
		highs: make([]decimal.Decimal, 0, cfg.LookbackBars),
		lows:  make([]decimal.Decimal, 0, cfg.LookbackBars),
	}
}

// OnMarketEvent processes a market event and generates signals.
func (b *Breakout) OnMarketEvent(ctx context.Context, event types.MarketEvent) []types.Signal {
	// Update high/low history
	b.highs = append(b.highs, event.High)
	b.lows = append(b.lows, event.Low)

	// Trim to lookback period
	if len(b.highs) > b.cfg.LookbackBars {
		b.highs = b.highs[1:]
		b.lows = b.lows[1:]
	}

	// Need enough history
	if len(b.highs) < b.cfg.LookbackBars {
		return nil
	}

	// Calculate range high/low (excluding current bar)
	rangeHigh := b.calculateHigh(b.highs[:len(b.highs)-1])
	rangeLow := b.calculateLow(b.lows[:len(b.lows)-1])

	// Check if range changed - reset signal flags
	if !rangeHigh.Equal(b.lastRangeHigh) || !rangeLow.Equal(b.lastRangeLow) {
		b.signalledLong = false
		b.signalledShort = false
		b.lastRangeHigh = rangeHigh
		b.lastRangeLow = rangeLow
	}

	// Check minimum ATR
	if !b.cfg.MinATR.IsZero() && event.ATR.LessThan(b.cfg.MinATR) {
		return nil
	}

	// Calculate breakout levels with buffer
	buffer := rangeHigh.Sub(rangeLow).Mul(b.cfg.BreakoutBuffer)
	breakoutHigh := rangeHigh.Add(buffer)
	breakoutLow := rangeLow.Sub(buffer)

	var signals []types.Signal

	// Check for breakout above
	if event.Close.GreaterThan(breakoutHigh) && !b.signalledLong {
		// Breakout above range - LONG signal
		signal := NewSignalBuilder(b.Name(), event).
			Long().
			WithATRStop(event.ATR, b.cfg.ATRMultiplier, getTickSize(event.Symbol)).
			WithReason(fmt.Sprintf("breakout above %.2f", breakoutHigh.InexactFloat64())).
			Build()
		signals = append(signals, signal)
		b.signalledLong = true
	}

	// Check for breakout below
	if event.Close.LessThan(breakoutLow) && !b.signalledShort {
		// Breakout below range - SHORT signal
		signal := NewSignalBuilder(b.Name(), event).
			Short().
			WithATRStop(event.ATR, b.cfg.ATRMultiplier, getTickSize(event.Symbol)).
			WithReason(fmt.Sprintf("breakout below %.2f", breakoutLow.InexactFloat64())).
			Build()
		signals = append(signals, signal)
		b.signalledShort = true
	}

	return signals
}

// Name returns the strategy name.
func (b *Breakout) Name() string {
	return "breakout"
}

// Reset clears all state.
func (b *Breakout) Reset() {
	b.highs = b.highs[:0]
	b.lows = b.lows[:0]
	b.signalledLong = false
	b.signalledShort = false
	b.lastRangeHigh = decimal.Zero
	b.lastRangeLow = decimal.Zero
	b.ready = false
}

// calculateHigh returns the highest value in the slice.
func (b *Breakout) calculateHigh(values []decimal.Decimal) decimal.Decimal {
	if len(values) == 0 {
		return decimal.Zero
	}
	high := values[0]
	for _, v := range values[1:] {
		if v.GreaterThan(high) {
			high = v
		}
	}
	return high
}

// calculateLow returns the lowest value in the slice.
func (b *Breakout) calculateLow(values []decimal.Decimal) decimal.Decimal {
	if len(values) == 0 {
		return decimal.Zero
	}
	low := values[0]
	for _, v := range values[1:] {
		if v.LessThan(low) {
			low = v
		}
	}
	return low
}

// getTickSize returns the tick size for a symbol.
func getTickSize(symbol string) decimal.Decimal {
	spec, ok := types.GetInstrumentSpec(symbol)
	if !ok {
		return decimal.RequireFromString("0.25") // Default to MES
	}
	return spec.TickSize
}
