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
	GridSpacingPoints decimal.Decimal // Distance between grid levels in points (e.g., 30 points)
	ReboundPct        decimal.Decimal // Take profit when price rebounds this % (e.g., 0.15 = 15% of move)
	MaxGridLevels     int             // Maximum number of open positions
	LookbackBars      int             // Bars to look back for swing high/low

	// Risk parameters
	StopLossPoints decimal.Decimal // Stop loss in points
	MinMovePoints  decimal.Decimal // Minimum price move to trigger grid entry

	// Cooldown
	CooldownBars int // Minimum bars between signals
}

// DefaultGridConfig returns sensible defaults for MES on M5/M15.
func DefaultGridConfig() GridConfig {
	return GridConfig{
		GridSpacingPoints: decimal.RequireFromString("25"),   // 25 points between levels
		ReboundPct:        decimal.RequireFromString("0.20"), // 20% rebound for TP
		MaxGridLevels:     3,                                  // Max 3 positions per direction
		LookbackBars:      30,                                 // 30 bars (~2.5 hours on M5)
		StopLossPoints:    decimal.RequireFromString("40"),   // 40 points stop loss
		MinMovePoints:     decimal.RequireFromString("20"),   // Min 20 points move to enter
		CooldownBars:      5,                                  // Wait 5 bars between signals
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
	swingHigh       decimal.Decimal
	swingLow        decimal.Decimal
	currentLevel    int        // Current grid level (0 = no position, 1-N = grid levels)
	lastSignalBar   int        // Bar index of last signal (for cooldown)
	barCount        int
	gridDirection   types.Side // LONG grid (buying dips) or SHORT grid (selling rallies)
	entryPrices     []decimal.Decimal // Track entry prices for each level
}

// NewGrid creates a new grid strategy.
func NewGrid(cfg GridConfig) *Grid {
	return &Grid{
		cfg:         cfg,
		highs:       make([]decimal.Decimal, 0, cfg.LookbackBars),
		lows:        make([]decimal.Decimal, 0, cfg.LookbackBars),
		entryPrices: make([]decimal.Decimal, 0, cfg.MaxGridLevels),
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

	// Check cooldown
	if g.barCount-g.lastSignalBar < g.cfg.CooldownBars {
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

	var signals []types.Signal

	// Calculate distances from extremes
	dropFromHigh := g.swingHigh.Sub(event.Close)
	riseFromLow := event.Close.Sub(g.swingLow)

	// Determine which direction to trade based on current position
	// If no active grid, pick the side with larger move
	if g.gridDirection == types.SideFlat {
		if dropFromHigh.GreaterThan(riseFromLow) && dropFromHigh.GreaterThan(g.cfg.MinMovePoints) {
			g.gridDirection = types.SideLong // Price dropped, look for longs
		} else if riseFromLow.GreaterThan(g.cfg.MinMovePoints) {
			g.gridDirection = types.SideShort // Price spiked, look for shorts
		}
	}

	// Process LONG grid (buying dips)
	if g.gridDirection == types.SideLong && dropFromHigh.GreaterThan(g.cfg.MinMovePoints) {
		// Calculate which grid level we should be at
		targetLevel := int(dropFromHigh.Div(g.cfg.GridSpacingPoints).IntPart()) + 1
		if targetLevel > g.cfg.MaxGridLevels {
			targetLevel = g.cfg.MaxGridLevels
		}

		// Only signal if we need to add a new level
		if targetLevel > g.currentLevel {
			// Calculate take profit (rebound of X% of the drop)
			reboundTarget := dropFromHigh.Mul(g.cfg.ReboundPct)
			tpPrice := event.Close.Add(reboundTarget)

			// Calculate stop loss
			stopPrice := event.Close.Sub(g.cfg.StopLossPoints)

			// Calculate stop in ticks
			tickSize := getTickSize(event.Symbol)
			stopTicks := int(g.cfg.StopLossPoints.Div(tickSize).Ceil().IntPart())

			signal := types.Signal{
				ID:           fmt.Sprintf("grid-L%d-%d", targetLevel, g.barCount),
				Timestamp:    event.Timestamp,
				Symbol:       event.Symbol,
				StrategyName: g.Name(),
				Direction:    types.SideLong,
				StopTicks:    stopTicks,
				Strength:     decimal.NewFromFloat(float64(targetLevel) / float64(g.cfg.MaxGridLevels)),
				Reason:       fmt.Sprintf("L%d drop=%.1f TP=%.2f SL=%.2f", targetLevel, dropFromHigh.InexactFloat64(), tpPrice.InexactFloat64(), stopPrice.InexactFloat64()),
			}
			signals = append(signals, signal)
			g.currentLevel = targetLevel
			g.lastSignalBar = g.barCount
			g.entryPrices = append(g.entryPrices, event.Close)
		}

		// Check if price rebounded enough to reset grid
		if g.currentLevel > 0 && len(g.entryPrices) > 0 {
			avgEntry := g.calculateAvgEntry()
			reboundFromEntry := event.Close.Sub(avgEntry)
			targetRebound := g.swingHigh.Sub(avgEntry).Mul(g.cfg.ReboundPct)
			if reboundFromEntry.GreaterThanOrEqual(targetRebound) {
				g.resetGrid()
			}
		}
	}

	// Process SHORT grid (selling rallies)
	if g.gridDirection == types.SideShort && riseFromLow.GreaterThan(g.cfg.MinMovePoints) {
		// Calculate which grid level we should be at
		targetLevel := int(riseFromLow.Div(g.cfg.GridSpacingPoints).IntPart()) + 1
		if targetLevel > g.cfg.MaxGridLevels {
			targetLevel = g.cfg.MaxGridLevels
		}

		// Only signal if we need to add a new level
		if targetLevel > g.currentLevel {
			// Calculate take profit (rebound of X% of the rise)
			reboundTarget := riseFromLow.Mul(g.cfg.ReboundPct)
			tpPrice := event.Close.Sub(reboundTarget)

			// Calculate stop loss
			stopPrice := event.Close.Add(g.cfg.StopLossPoints)

			// Calculate stop in ticks
			tickSize := getTickSize(event.Symbol)
			stopTicks := int(g.cfg.StopLossPoints.Div(tickSize).Ceil().IntPart())

			signal := types.Signal{
				ID:           fmt.Sprintf("grid-S%d-%d", targetLevel, g.barCount),
				Timestamp:    event.Timestamp,
				Symbol:       event.Symbol,
				StrategyName: g.Name(),
				Direction:    types.SideShort,
				StopTicks:    stopTicks,
				Strength:     decimal.NewFromFloat(float64(targetLevel) / float64(g.cfg.MaxGridLevels)),
				Reason:       fmt.Sprintf("S%d rise=%.1f TP=%.2f SL=%.2f", targetLevel, riseFromLow.InexactFloat64(), tpPrice.InexactFloat64(), stopPrice.InexactFloat64()),
			}
			signals = append(signals, signal)
			g.currentLevel = targetLevel
			g.lastSignalBar = g.barCount
			g.entryPrices = append(g.entryPrices, event.Close)
		}

		// Check if price rebounded enough to reset grid
		if g.currentLevel > 0 && len(g.entryPrices) > 0 {
			avgEntry := g.calculateAvgEntry()
			reboundFromEntry := avgEntry.Sub(event.Close)
			targetRebound := avgEntry.Sub(g.swingLow).Mul(g.cfg.ReboundPct)
			if reboundFromEntry.GreaterThanOrEqual(targetRebound) {
				g.resetGrid()
			}
		}
	}

	// Reset grid when price returns to middle of range (no active grid)
	if g.currentLevel == 0 {
		midPoint := g.swingLow.Add(swingRange.Div(decimal.NewFromInt(2)))
		distanceToMid := event.Close.Sub(midPoint).Abs()
		if distanceToMid.LessThan(g.cfg.GridSpacingPoints) {
			g.gridDirection = types.SideFlat
		}
	}

	return signals
}

// calculateAvgEntry returns the average entry price of current positions.
func (g *Grid) calculateAvgEntry() decimal.Decimal {
	if len(g.entryPrices) == 0 {
		return decimal.Zero
	}
	sum := decimal.Zero
	for _, p := range g.entryPrices {
		sum = sum.Add(p)
	}
	return sum.Div(decimal.NewFromInt(int64(len(g.entryPrices))))
}

// resetGrid clears the current grid state.
func (g *Grid) resetGrid() {
	g.currentLevel = 0
	g.gridDirection = types.SideFlat
	g.entryPrices = g.entryPrices[:0]
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
	g.currentLevel = 0
	g.lastSignalBar = 0
	g.barCount = 0
	g.gridDirection = types.SideFlat
	g.entryPrices = g.entryPrices[:0]
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
