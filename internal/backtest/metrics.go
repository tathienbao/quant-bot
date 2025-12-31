package backtest

import (
	"math"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Metrics provides advanced performance metrics calculations.
type Metrics struct {
	trades      []types.Trade
	equityCurve []EquityPoint
	riskFreeRate decimal.Decimal // Annual risk-free rate (e.g., 0.05 for 5%)
}

// NewMetrics creates a new metrics calculator.
func NewMetrics(result *Result, riskFreeRate decimal.Decimal) *Metrics {
	return &Metrics{
		trades:       result.Trades,
		equityCurve:  result.EquityCurve,
		riskFreeRate: riskFreeRate,
	}
}

// SharpeRatio calculates the annualized Sharpe ratio.
// Sharpe = (mean_return - risk_free) / std_dev_returns * sqrt(252)
func (m *Metrics) SharpeRatio() decimal.Decimal {
	returns := m.calculateReturns()
	if len(returns) < 2 {
		return decimal.Zero
	}

	meanReturn := mean(returns)
	stdDev := standardDeviation(returns)

	if stdDev.IsZero() {
		return decimal.Zero
	}

	// Daily risk-free rate (assuming 252 trading days)
	dailyRf := m.riskFreeRate.Div(decimal.NewFromInt(252))
	excessReturn := meanReturn.Sub(dailyRf)

	// Annualize: multiply by sqrt(252)
	sqrt252 := decimal.NewFromFloat(math.Sqrt(252))
	sharpe := excessReturn.Div(stdDev).Mul(sqrt252)

	return sharpe
}

// SortinoRatio calculates the Sortino ratio (uses downside deviation).
// Sortino = (mean_return - risk_free) / downside_deviation * sqrt(252)
func (m *Metrics) SortinoRatio() decimal.Decimal {
	returns := m.calculateReturns()
	if len(returns) < 2 {
		return decimal.Zero
	}

	meanReturn := mean(returns)
	downsideDev := downsideDeviation(returns, decimal.Zero)

	if downsideDev.IsZero() {
		return decimal.Zero
	}

	dailyRf := m.riskFreeRate.Div(decimal.NewFromInt(252))
	excessReturn := meanReturn.Sub(dailyRf)

	sqrt252 := decimal.NewFromFloat(math.Sqrt(252))
	sortino := excessReturn.Div(downsideDev).Mul(sqrt252)

	return sortino
}

// MaxDrawdown returns the maximum drawdown as a ratio.
func (m *Metrics) MaxDrawdown() decimal.Decimal {
	if len(m.equityCurve) == 0 {
		return decimal.Zero
	}

	hwm := m.equityCurve[0].Equity
	maxDD := decimal.Zero

	for _, point := range m.equityCurve {
		if point.Equity.GreaterThan(hwm) {
			hwm = point.Equity
		}
		if hwm.IsPositive() {
			dd := hwm.Sub(point.Equity).Div(hwm)
			if dd.GreaterThan(maxDD) {
				maxDD = dd
			}
		}
	}

	return maxDD
}

// CalmarRatio calculates the Calmar ratio (annual return / max drawdown).
func (m *Metrics) CalmarRatio() decimal.Decimal {
	maxDD := m.MaxDrawdown()
	if maxDD.IsZero() {
		return decimal.Zero
	}

	annualReturn := m.AnnualizedReturn()
	return annualReturn.Div(maxDD)
}

// AnnualizedReturn calculates the annualized return.
func (m *Metrics) AnnualizedReturn() decimal.Decimal {
	if len(m.equityCurve) < 2 {
		return decimal.Zero
	}

	first := m.equityCurve[0]
	last := m.equityCurve[len(m.equityCurve)-1]

	if first.Equity.IsZero() {
		return decimal.Zero
	}

	totalReturn := last.Equity.Sub(first.Equity).Div(first.Equity)

	// Calculate days
	days := last.Timestamp.Sub(first.Timestamp).Hours() / 24
	if days <= 0 {
		return totalReturn
	}

	// Annualize: (1 + total_return)^(365/days) - 1
	yearsFloat := days / 365
	if yearsFloat < 0.01 { // Less than ~4 days
		return totalReturn
	}

	totalReturnFloat := totalReturn.InexactFloat64()
	annualizedFloat := math.Pow(1+totalReturnFloat, 1/yearsFloat) - 1

	return decimal.NewFromFloat(annualizedFloat)
}

// WinRate returns the win rate as a ratio.
func (m *Metrics) WinRate() decimal.Decimal {
	if len(m.trades) == 0 {
		return decimal.Zero
	}

	wins := 0
	for _, trade := range m.trades {
		if trade.NetPL.IsPositive() {
			wins++
		}
	}

	return decimal.NewFromInt(int64(wins)).Div(decimal.NewFromInt(int64(len(m.trades))))
}

// ProfitFactor calculates gross profit / gross loss.
func (m *Metrics) ProfitFactor() decimal.Decimal {
	grossProfit := decimal.Zero
	grossLoss := decimal.Zero

	for _, trade := range m.trades {
		if trade.NetPL.IsPositive() {
			grossProfit = grossProfit.Add(trade.NetPL)
		} else {
			grossLoss = grossLoss.Add(trade.NetPL.Abs())
		}
	}

	if grossLoss.IsZero() {
		return decimal.Zero
	}

	return grossProfit.Div(grossLoss)
}

// AverageWin returns the average winning trade P&L.
func (m *Metrics) AverageWin() decimal.Decimal {
	totalWin := decimal.Zero
	winCount := 0

	for _, trade := range m.trades {
		if trade.NetPL.IsPositive() {
			totalWin = totalWin.Add(trade.NetPL)
			winCount++
		}
	}

	if winCount == 0 {
		return decimal.Zero
	}

	return totalWin.Div(decimal.NewFromInt(int64(winCount)))
}

// AverageLoss returns the average losing trade P&L.
func (m *Metrics) AverageLoss() decimal.Decimal {
	totalLoss := decimal.Zero
	lossCount := 0

	for _, trade := range m.trades {
		if trade.NetPL.IsNegative() {
			totalLoss = totalLoss.Add(trade.NetPL)
			lossCount++
		}
	}

	if lossCount == 0 {
		return decimal.Zero
	}

	return totalLoss.Div(decimal.NewFromInt(int64(lossCount)))
}

// Expectancy calculates expected value per trade.
// Expectancy = (WinRate * AvgWin) + ((1 - WinRate) * AvgLoss)
func (m *Metrics) Expectancy() decimal.Decimal {
	winRate := m.WinRate()
	avgWin := m.AverageWin()
	avgLoss := m.AverageLoss() // Negative

	return winRate.Mul(avgWin).Add(decimal.NewFromInt(1).Sub(winRate).Mul(avgLoss))
}

// calculateReturns computes daily returns from equity curve.
func (m *Metrics) calculateReturns() []decimal.Decimal {
	if len(m.equityCurve) < 2 {
		return nil
	}

	returns := make([]decimal.Decimal, 0, len(m.equityCurve)-1)
	for i := 1; i < len(m.equityCurve); i++ {
		prev := m.equityCurve[i-1].Equity
		curr := m.equityCurve[i].Equity

		if prev.IsZero() {
			continue
		}

		ret := curr.Sub(prev).Div(prev)
		returns = append(returns, ret)
	}

	return returns
}

// Helper: mean of decimal slice.
func mean(values []decimal.Decimal) decimal.Decimal {
	if len(values) == 0 {
		return decimal.Zero
	}

	sum := decimal.Zero
	for _, v := range values {
		sum = sum.Add(v)
	}

	return sum.Div(decimal.NewFromInt(int64(len(values))))
}

// Helper: standard deviation of decimal slice.
func standardDeviation(values []decimal.Decimal) decimal.Decimal {
	if len(values) < 2 {
		return decimal.Zero
	}

	m := mean(values)
	sumSquares := decimal.Zero

	for _, v := range values {
		diff := v.Sub(m)
		sumSquares = sumSquares.Add(diff.Mul(diff))
	}

	variance := sumSquares.Div(decimal.NewFromInt(int64(len(values) - 1)))

	// sqrt using float conversion
	varianceFloat := variance.InexactFloat64()
	if varianceFloat < 0 {
		return decimal.Zero
	}

	return decimal.NewFromFloat(math.Sqrt(varianceFloat))
}

// Helper: downside deviation (std dev of negative returns only).
func downsideDeviation(returns []decimal.Decimal, target decimal.Decimal) decimal.Decimal {
	negativeReturns := make([]decimal.Decimal, 0)

	for _, r := range returns {
		if r.LessThan(target) {
			negativeReturns = append(negativeReturns, r)
		}
	}

	if len(negativeReturns) < 2 {
		return decimal.Zero
	}

	return standardDeviation(negativeReturns)
}
