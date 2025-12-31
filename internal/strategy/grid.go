package strategy

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// GridConfig holds configuration for the grid/rebound strategy.
type GridConfig struct {
	// Grid parameters
	GridSpacingPct    decimal.Decimal // Distance between grid levels as % of price (e.g., 0.002 = 0.2%)
	ReboundPct        decimal.Decimal // Take profit when price rebounds this % (e.g., 0.15 = 15% of move)
	MaxGridLevels     int             // Maximum number of open positions
	LookbackBars      int             // Bars to look back for swing high/low

	// Risk parameters
	StopLossPct       decimal.Decimal // Stop loss as % of entry (e.g., 0.01 = 1%)
	MinMovePoints     decimal.Decimal // Minimum price move to trigger grid entry
}

// DefaultGridConfig returns sensible defaults for MES on M5/M15.
func DefaultGridConfig() GridConfig {
	return GridConfig{
		GridSpacingPct:    decimal.RequireFromString("0.002"),  // 0.2% between levels
		ReboundPct:        decimal.RequireFromString("0.15"),   // 15% rebound for TP
		MaxGridLevels:     5,                                    // Max 5 positions
		LookbackBars:      20,                                   // 20 bars for swing detection
		StopLossPct:       decimal.RequireFromString("0.005"),  // 0.5% stop loss
		MinMovePoints:     decimal.RequireFromString("10"),     // Min 10 points move
	}
}

// Grid implements a grid/rebound trading strategy.
// It enters counter-trend positions when price moves significantly,
// expecting a rebound of 10-20% of the move.
//
// Core logic (from experienced trader):
// - "Raw math" - follows price, not chart patterns
// - Places orders at calculated intervals (grid)
// - Only needs 10-20% rebound to take profit
// - Designed for intraday timeframes (M5/M15)
type Grid struct {
	cfg GridConfig

	// Price history
	highs []decimal.Decimal
	lows  []decimal.Decimal

	// Current grid state
	swingHigh     decimal.Decimal
	swingLow      decimal.Decimal
	lastGridLevel int // Current grid level (0 = no position, 1-N = grid levels)
	lastSignalBar int // Bar index of last signal (prevent rapid firing)
	barCount      int

	// Track active grid direction
	gridDirection types.Side // LONG grid (buying dips) or SHORT grid (selling rallies)
}

// NewGrid creates a new grid strategy.
func NewGrid(cfg GridConfig) *Grid {
	return &Grid{
		cfg:   cfg,
		highs: make([]decimal.Decimal, 0, cfg.LookbackBars),
		lows:  make([]decimal.Decimal, 0, cfg.LookbackBars),
	}
}

// OnMarketEvent processes a market event and generates signals.
func (g *Grid) OnMarketEvent(ctx context.Context, event types.MarketEvent) []types.Signal {
	g.barCount++

	// Update price history
	g.highs = append(g.highs, event.High)
	g.lows = append(g.lows, event.Low)

	// Trim to lookback period
	if len(g.highs) > g.cfg.LookbackBars {
		g.highs = g.highs[1:]
		g.lows = g.lows[1:]
	}

	// Need enough history
	if len(g.highs) < g.cfg.LookbackBars {
		return nil
	}

	// Calculate swing high/low
	g.swingHigh = g.calculateHigh(g.highs)
	g.swingLow = g.calculateLow(g.lows)

	// Calculate the swing range
	swingRange := g.swingHigh.Sub(g.swingLow)
	if swingRange.LessThan(g.cfg.MinMovePoints) {
		return nil // Range too small, no grid opportunity
	}

	// Calculate grid spacing in points
	gridSpacing := event.Close.Mul(g.cfg.GridSpacingPct)

	var signals []types.Signal

	// Check for LONG grid opportunity (price dropped from high)
	dropFromHigh := g.swingHigh.Sub(event.Close)
	if dropFromHigh.GreaterThan(g.cfg.MinMovePoints) {
		// Calculate which grid level we're at
		gridLevel := int(dropFromHigh.Div(gridSpacing).IntPart()) + 1

		// Only signal if we've moved to a new grid level
		if gridLevel > g.lastGridLevel && gridLevel <= g.cfg.MaxGridLevels {
			// Calculate take profit (rebound of 10-20% of the drop)
			reboundTarget := dropFromHigh.Mul(g.cfg.ReboundPct)
			tpPrice := event.Close.Add(reboundTarget)

			// Calculate stop loss
			stopDistance := event.Close.Mul(g.cfg.StopLossPct)
			stopPrice := event.Close.Sub(stopDistance)

			// Calculate stop in ticks
			tickSize := getTickSize(event.Symbol)
			stopTicks := int(stopDistance.Div(tickSize).Ceil().IntPart())

			signal := types.Signal{
				ID:           fmt.Sprintf("grid-long-%d-%d", g.barCount, gridLevel),
				Timestamp:    event.Timestamp,
				Symbol:       event.Symbol,
				StrategyName: g.Name(),
				Direction:    types.SideLong,
				StopTicks:    stopTicks,
				Strength:     decimal.NewFromFloat(float64(gridLevel) / float64(g.cfg.MaxGridLevels)),
				Reason:       fmt.Sprintf("grid L%d: drop %.2f pts, TP %.2f, SL %.2f", gridLevel, dropFromHigh.InexactFloat64(), tpPrice.InexactFloat64(), stopPrice.InexactFloat64()),
			}
			signals = append(signals, signal)
			g.lastGridLevel = gridLevel
			g.gridDirection = types.SideLong
		}
	}

	// Check for SHORT grid opportunity (price spiked from low)
	riseFromLow := event.Close.Sub(g.swingLow)
	if riseFromLow.GreaterThan(g.cfg.MinMovePoints) && g.gridDirection != types.SideLong {
		// Calculate which grid level we're at
		gridLevel := int(riseFromLow.Div(gridSpacing).IntPart()) + 1

		// Only signal if we've moved to a new grid level
		if gridLevel > g.lastGridLevel && gridLevel <= g.cfg.MaxGridLevels {
			// Calculate take profit (rebound of 10-20% of the rise)
			reboundTarget := riseFromLow.Mul(g.cfg.ReboundPct)
			tpPrice := event.Close.Sub(reboundTarget)

			// Calculate stop loss
			stopDistance := event.Close.Mul(g.cfg.StopLossPct)
			stopPrice := event.Close.Add(stopDistance)

			// Calculate stop in ticks
			tickSize := getTickSize(event.Symbol)
			stopTicks := int(stopDistance.Div(tickSize).Ceil().IntPart())

			signal := types.Signal{
				ID:           fmt.Sprintf("grid-short-%d-%d", g.barCount, gridLevel),
				Timestamp:    event.Timestamp,
				Symbol:       event.Symbol,
				StrategyName: g.Name(),
				Direction:    types.SideShort,
				StopTicks:    stopTicks,
				Strength:     decimal.NewFromFloat(float64(gridLevel) / float64(g.cfg.MaxGridLevels)),
				Reason:       fmt.Sprintf("grid S%d: rise %.2f pts, TP %.2f, SL %.2f", gridLevel, riseFromLow.InexactFloat64(), tpPrice.InexactFloat64(), stopPrice.InexactFloat64()),
			}
			signals = append(signals, signal)
			g.lastGridLevel = gridLevel
			g.gridDirection = types.SideShort
		}
	}

	// Reset grid when price returns to middle of range
	midPoint := g.swingLow.Add(swingRange.Div(decimal.NewFromInt(2)))
	distanceToMid := event.Close.Sub(midPoint).Abs()
	if distanceToMid.LessThan(gridSpacing) {
		g.lastGridLevel = 0
		g.gridDirection = types.SideFlat
	}

	return signals
}

// Name returns the strategy name.
func (g *Grid) Name() string {
	return "grid"
}

// Reset clears all state.
func (g *Grid) Reset() {
	g.highs = g.highs[:0]
	g.lows = g.lows[:0]
	g.swingHigh = decimal.Zero
	g.swingLow = decimal.Zero
	g.lastGridLevel = 0
	g.lastSignalBar = 0
	g.barCount = 0
	g.gridDirection = types.SideFlat
}

// calculateHigh returns the highest value in the slice.
func (g *Grid) calculateHigh(values []decimal.Decimal) decimal.Decimal {
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
func (g *Grid) calculateLow(values []decimal.Decimal) decimal.Decimal {
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
