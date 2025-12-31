package alerting

import (
	"time"

	"github.com/shopspring/decimal"
)

// DailySummary contains daily trading statistics for the summary report.
type DailySummary struct {
	Date           time.Time
	StartingEquity decimal.Decimal
	EndingEquity   decimal.Decimal
	HighWaterMark  decimal.Decimal
	TotalPL        decimal.Decimal
	ReturnPct      decimal.Decimal
	Drawdown       decimal.Decimal
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        decimal.Decimal
	SafeModeActive bool
	OpenPositions  int
}

// NewDailySummary creates a new daily summary from the provided data.
func NewDailySummary(
	date time.Time,
	startEquity, endEquity, highWater decimal.Decimal,
	totalTrades, winningTrades, losingTrades int,
	safeModeActive bool,
	openPositions int,
) DailySummary {
	totalPL := endEquity.Sub(startEquity)

	var returnPct decimal.Decimal
	if !startEquity.IsZero() {
		returnPct = totalPL.Div(startEquity).Mul(decimal.NewFromInt(100))
	}

	var drawdown decimal.Decimal
	if !highWater.IsZero() {
		drawdown = highWater.Sub(endEquity).Div(highWater).Mul(decimal.NewFromInt(100))
		if drawdown.IsNegative() {
			drawdown = decimal.Zero
		}
	}

	var winRate decimal.Decimal
	if totalTrades > 0 {
		winRate = decimal.NewFromInt(int64(winningTrades)).
			Div(decimal.NewFromInt(int64(totalTrades))).
			Mul(decimal.NewFromInt(100))
	}

	return DailySummary{
		Date:           date,
		StartingEquity: startEquity,
		EndingEquity:   endEquity,
		HighWaterMark:  highWater,
		TotalPL:        totalPL,
		ReturnPct:      returnPct,
		Drawdown:       drawdown,
		TotalTrades:    totalTrades,
		WinningTrades:  winningTrades,
		LosingTrades:   losingTrades,
		WinRate:        winRate,
		SafeModeActive: safeModeActive,
		OpenPositions:  openPositions,
	}
}
