package risk

import (
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// PositionSizer calculates position size based on risk parameters.
type PositionSizer struct {
	tickValue decimal.Decimal
}

// NewPositionSizer creates a new position sizer for a given instrument.
func NewPositionSizer(tickValue decimal.Decimal) *PositionSizer {
	return &PositionSizer{
		tickValue: tickValue,
	}
}

// NewPositionSizerForSymbol creates a position sizer for a known symbol.
func NewPositionSizerForSymbol(symbol string) (*PositionSizer, error) {
	spec, ok := types.GetInstrumentSpec(symbol)
	if !ok {
		return nil, types.ErrInvalidSymbol
	}
	return NewPositionSizer(spec.TickValue), nil
}

// SizeResult contains the result of position size calculation.
type SizeResult struct {
	Contracts    int             // Number of contracts to trade
	RiskAmount   decimal.Decimal // Actual dollar risk
	StopLoss     decimal.Decimal // Stop loss price (if entry provided)
	Valid        bool            // Whether the calculation is valid
	RejectReason string          // Reason if not valid
}

// Calculate determines the position size based on risk parameters.
//
// Formula:
//
//	capital_at_risk = equity * riskPerTradePct
//	tick_risk = stopDistanceTicks * tickValue
//	contracts = floor(capital_at_risk / tick_risk)
//
// Returns 0 contracts if the calculated size is less than 1.
func (p *PositionSizer) Calculate(
	equity decimal.Decimal,
	riskPerTradePct decimal.Decimal,
	stopDistanceTicks int,
) int {
	if stopDistanceTicks <= 0 {
		return 0
	}

	if equity.LessThanOrEqual(decimal.Zero) {
		return 0
	}

	if riskPerTradePct.LessThanOrEqual(decimal.Zero) {
		return 0
	}

	// capital_at_risk = equity * risk_per_trade_pct
	capitalAtRisk := equity.Mul(riskPerTradePct)

	// tick_risk = stop_distance_ticks * tick_value
	tickRisk := decimal.NewFromInt(int64(stopDistanceTicks)).Mul(p.tickValue)

	if tickRisk.IsZero() {
		return 0
	}

	// contracts = floor(capital_at_risk / tick_risk)
	contracts := capitalAtRisk.Div(tickRisk).Floor()

	// Convert to int, ensuring non-negative
	contractsInt := int(contracts.IntPart())
	if contractsInt < 0 {
		return 0
	}

	return contractsInt
}

// CalculateWithDetails provides full calculation details.
func (p *PositionSizer) CalculateWithDetails(
	equity decimal.Decimal,
	riskPerTradePct decimal.Decimal,
	stopDistanceTicks int,
	entryPrice decimal.Decimal,
	side types.Side,
	tickSize decimal.Decimal,
) SizeResult {
	result := SizeResult{}

	// Validation
	if stopDistanceTicks <= 0 {
		result.RejectReason = "stop distance must be positive"
		return result
	}

	if equity.LessThanOrEqual(decimal.Zero) {
		result.RejectReason = "equity must be positive"
		return result
	}

	if riskPerTradePct.LessThanOrEqual(decimal.Zero) {
		result.RejectReason = "risk per trade must be positive"
		return result
	}

	if riskPerTradePct.GreaterThan(decimal.RequireFromString("0.1")) {
		result.RejectReason = "risk per trade exceeds 10% maximum"
		return result
	}

	// Calculate position size
	contracts := p.Calculate(equity, riskPerTradePct, stopDistanceTicks)

	if contracts < 1 {
		result.RejectReason = "calculated position size less than 1 contract"
		return result
	}

	// Calculate actual risk amount
	tickRisk := decimal.NewFromInt(int64(stopDistanceTicks)).Mul(p.tickValue)
	result.RiskAmount = tickRisk.Mul(decimal.NewFromInt(int64(contracts)))

	// Calculate stop loss price
	stopDistance := tickSize.Mul(decimal.NewFromInt(int64(stopDistanceTicks)))
	switch side {
	case types.SideLong:
		result.StopLoss = entryPrice.Sub(stopDistance)
	case types.SideShort:
		result.StopLoss = entryPrice.Add(stopDistance)
	}

	result.Contracts = contracts
	result.Valid = true

	return result
}

// MaxContracts calculates the maximum contracts allowed given exposure limits.
func (p *PositionSizer) MaxContracts(
	equity decimal.Decimal,
	maxExposurePct decimal.Decimal,
	currentPrice decimal.Decimal,
	pointValue decimal.Decimal,
) int {
	if currentPrice.IsZero() || pointValue.IsZero() {
		return 0
	}

	// Max exposure in dollars
	maxExposure := equity.Mul(maxExposurePct)

	// Notional value per contract = price * point_value
	notionalPerContract := currentPrice.Mul(pointValue)

	if notionalPerContract.IsZero() {
		return 0
	}

	// Max contracts = max_exposure / notional_per_contract
	maxContracts := maxExposure.Div(notionalPerContract).Floor()

	return int(maxContracts.IntPart())
}

// AdjustForMaxSize ensures position size doesn't exceed maximum.
func (p *PositionSizer) AdjustForMaxSize(calculated int, maxAllowed int) int {
	if calculated > maxAllowed {
		return maxAllowed
	}
	return calculated
}
