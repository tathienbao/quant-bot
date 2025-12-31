package observer

import (
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
	"github.com/tathienbao/quant-bot/pkg/indicator"
)

// CalculatorConfig holds configuration for the indicator calculator.
type CalculatorConfig struct {
	ATRPeriod    int // Period for ATR calculation
	StdDevPeriod int // Period for StdDev calculation
	SMAPeriod    int // Period for SMA calculation (optional)
}

// DefaultCalculatorConfig returns sensible defaults.
func DefaultCalculatorConfig() CalculatorConfig {
	return CalculatorConfig{
		ATRPeriod:    14,
		StdDevPeriod: 20,
		SMAPeriod:    20,
	}
}

// Calculator calculates technical indicators for market events.
type Calculator struct {
	cfg    CalculatorConfig
	atr    *indicator.ATR
	stddev *indicator.StdDev
	sma    *indicator.SMA
}

// NewCalculator creates a new indicator calculator.
func NewCalculator(cfg CalculatorConfig) *Calculator {
	return &Calculator{
		cfg:    cfg,
		atr:    indicator.NewATR(cfg.ATRPeriod),
		stddev: indicator.NewStdDev(cfg.StdDevPeriod),
		sma:    indicator.NewSMA(cfg.SMAPeriod),
	}
}

// OnBar processes a new bar and updates all indicators.
// Returns the MarketEvent enriched with calculated indicators.
func (c *Calculator) OnBar(event types.MarketEvent) types.MarketEvent {
	// Update ATR with OHLC
	atr := c.atr.Update(event.High, event.Low, event.Close)

	// Update StdDev with close price
	stddev := c.stddev.Update(event.Close)

	// Update SMA with close price
	c.sma.Update(event.Close)

	// Enrich event with indicators
	event.ATR = atr
	event.StdDev = stddev

	return event
}

// Reset clears all indicator state.
func (c *Calculator) Reset() {
	c.atr.Reset()
	c.stddev.Reset()
	c.sma.Reset()
}

// Ready returns true if all indicators have enough data.
func (c *Calculator) Ready() bool {
	return c.atr.Ready() && c.stddev.Ready()
}

// CurrentATR returns the current ATR value.
func (c *Calculator) CurrentATR() decimal.Decimal {
	return c.atr.Current()
}

// CurrentStdDev returns the current StdDev value.
func (c *Calculator) CurrentStdDev() decimal.Decimal {
	return c.stddev.Current()
}

// CurrentSMA returns the current SMA value.
func (c *Calculator) CurrentSMA() decimal.Decimal {
	return c.sma.Current()
}
