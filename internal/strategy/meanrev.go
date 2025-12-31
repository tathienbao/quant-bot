package strategy

/*
╔══════════════════════════════════════════════════════════════════════════════╗
║                      MEAN REVERSION STRATEGY                                  ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Backtest Results (MES, $100k equity, 1% risk/trade)                         ║
╠═══════════════════╦═══════════════════════╦══════════════════════════════════╣
║  Data             ║  Daily (1 năm)        ║  M5 (2.5 tháng)                  ║
╠═══════════════════╬═══════════════════════╬══════════════════════════════════╣
║  Return           ║  -3.62%               ║  -15.53%                         ║
║  Max Drawdown     ║  8.90%                ║  20.25%                          ║
║  Total Trades     ║  15                   ║  53                              ║
║  Win Rate         ║  20%                  ║  24.53%                          ║
║  Profit Factor    ║  0.59                 ║  0.66                            ║
╠═══════════════════╩═══════════════════════╩══════════════════════════════════╣
║  ⚠️  KHÔNG KHUYẾN NGHỊ - Strategy này thua lỗ trên cả 2 timeframe            ║
╚══════════════════════════════════════════════════════════════════════════════╝
*/

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
	"github.com/tathienbao/quant-bot/pkg/indicator"
)

// MeanRevConfig holds configuration for the mean reversion strategy.
type MeanRevConfig struct {
	SMAPeriod       int             // Period for SMA calculation
	StdDevPeriod    int             // Period for StdDev calculation
	EntryStdDev     decimal.Decimal // Number of StdDevs from mean to enter (e.g., 2.0)
	ATRMultiplier   decimal.Decimal // ATR multiplier for stop loss
	MinStdDev       decimal.Decimal // Minimum StdDev to generate signal
}

// DefaultMeanRevConfig returns sensible defaults.
func DefaultMeanRevConfig() MeanRevConfig {
	return MeanRevConfig{
		SMAPeriod:     20,
		StdDevPeriod:  20,
		EntryStdDev:   decimal.RequireFromString("2.0"),
		ATRMultiplier: decimal.RequireFromString("1.5"),
		MinStdDev:     decimal.Zero,
	}
}

// MeanReversion implements a mean reversion strategy.
// Generates LONG when price is below SMA - (EntryStdDev * StdDev).
// Generates SHORT when price is above SMA + (EntryStdDev * StdDev).
type MeanReversion struct {
	cfg    MeanRevConfig
	sma    *indicator.SMA
	stddev *indicator.StdDev

	lastSignalUp   bool // Prevent repeated signals
	lastSignalDown bool
}

// NewMeanReversion creates a new mean reversion strategy.
func NewMeanReversion(cfg MeanRevConfig) *MeanReversion {
	return &MeanReversion{
		cfg:    cfg,
		sma:    indicator.NewSMA(cfg.SMAPeriod),
		stddev: indicator.NewStdDev(cfg.StdDevPeriod),
	}
}

// OnMarketEvent processes a market event and generates signals.
func (m *MeanReversion) OnMarketEvent(ctx context.Context, event types.MarketEvent) []types.Signal {
	// Get current mean and stddev BEFORE updating (for signal generation)
	prevMean := m.sma.Current()
	prevStdDev := m.stddev.Current()
	wasReady := m.sma.Ready() && m.stddev.Ready()

	// Update indicators with new bar
	m.sma.Update(event.Close)
	m.stddev.Update(event.Close)

	// Need previous values to generate signals
	if !wasReady {
		return nil
	}

	// Check minimum StdDev
	if !m.cfg.MinStdDev.IsZero() && prevStdDev.LessThan(m.cfg.MinStdDev) {
		return nil
	}

	// Calculate bands using previous mean/stddev
	deviation := prevStdDev.Mul(m.cfg.EntryStdDev)
	upperBand := prevMean.Add(deviation)
	lowerBand := prevMean.Sub(deviation)

	var signals []types.Signal

	// Check for mean reversion signals
	if event.Close.LessThan(lowerBand) && !m.lastSignalDown {
		// Price below lower band - LONG signal (expect reversion up)
		signal := NewSignalBuilder(m.Name(), event).
			Long().
			WithATRStop(event.ATR, m.cfg.ATRMultiplier, getTickSize(event.Symbol)).
			WithReason(fmt.Sprintf("price %.2f below lower band %.2f",
				event.Close.InexactFloat64(), lowerBand.InexactFloat64())).
			Build()
		signals = append(signals, signal)
		m.lastSignalDown = true
		m.lastSignalUp = false
	} else if event.Close.GreaterThan(upperBand) && !m.lastSignalUp {
		// Price above upper band - SHORT signal (expect reversion down)
		signal := NewSignalBuilder(m.Name(), event).
			Short().
			WithATRStop(event.ATR, m.cfg.ATRMultiplier, getTickSize(event.Symbol)).
			WithReason(fmt.Sprintf("price %.2f above upper band %.2f",
				event.Close.InexactFloat64(), upperBand.InexactFloat64())).
			Build()
		signals = append(signals, signal)
		m.lastSignalUp = true
		m.lastSignalDown = false
	} else if event.Close.GreaterThan(lowerBand) && event.Close.LessThan(upperBand) {
		// Price back within bands - reset signal flags
		m.lastSignalUp = false
		m.lastSignalDown = false
	}

	return signals
}

// Name returns the strategy name.
func (m *MeanReversion) Name() string {
	return "meanrev"
}

// Reset clears all state.
func (m *MeanReversion) Reset() {
	m.sma.Reset()
	m.stddev.Reset()
	m.lastSignalUp = false
	m.lastSignalDown = false
}

// CurrentMean returns the current SMA value.
func (m *MeanReversion) CurrentMean() decimal.Decimal {
	return m.sma.Current()
}

// CurrentStdDev returns the current StdDev value.
func (m *MeanReversion) CurrentStdDev() decimal.Decimal {
	return m.stddev.Current()
}

// Bands returns the current upper and lower bands.
func (m *MeanReversion) Bands() (upper, lower decimal.Decimal) {
	mean := m.sma.Current()
	stddev := m.stddev.Current()
	deviation := stddev.Mul(m.cfg.EntryStdDev)
	return mean.Add(deviation), mean.Sub(deviation)
}
