// Package types defines shared types used across the trading system.
package types

import (
	"time"

	"github.com/shopspring/decimal"
)

// Side represents the direction of a trade.
type Side int

const (
	SideFlat Side = iota
	SideLong
	SideShort
)

func (s Side) String() string {
	switch s {
	case SideLong:
		return "LONG"
	case SideShort:
		return "SHORT"
	default:
		return "FLAT"
	}
}

// Opposite returns the opposite side.
func (s Side) Opposite() Side {
	switch s {
	case SideLong:
		return SideShort
	case SideShort:
		return SideLong
	default:
		return SideFlat
	}
}

// OrderStatus represents the state of an order.
type OrderStatus int

const (
	OrderStatusCreated OrderStatus = iota
	OrderStatusPending
	OrderStatusPartialFill
	OrderStatusFilled
	OrderStatusRejected
	OrderStatusCancelled
	OrderStatusExpired
)

func (s OrderStatus) String() string {
	switch s {
	case OrderStatusCreated:
		return "CREATED"
	case OrderStatusPending:
		return "PENDING"
	case OrderStatusPartialFill:
		return "PARTIAL_FILL"
	case OrderStatusFilled:
		return "FILLED"
	case OrderStatusRejected:
		return "REJECTED"
	case OrderStatusCancelled:
		return "CANCELLED"
	case OrderStatusExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

// IsFinal returns true if the order is in a terminal state.
func (s OrderStatus) IsFinal() bool {
	switch s {
	case OrderStatusFilled, OrderStatusRejected, OrderStatusCancelled, OrderStatusExpired:
		return true
	default:
		return false
	}
}

// MarketEvent represents a market data update.
type MarketEvent struct {
	Symbol    string
	Timestamp time.Time
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    int64
	ATR       decimal.Decimal // Average True Range
	StdDev    decimal.Decimal // Standard Deviation
}

// Signal represents a trading signal from a strategy.
type Signal struct {
	ID            string
	Timestamp     time.Time
	Symbol        string
	Direction     Side
	Strength      decimal.Decimal // 0-1, optional confidence
	StopTicks     int             // Suggested stop distance in ticks
	TakeProfitATR decimal.Decimal // Take profit as ATR multiple
	Reason        string          // Why this signal was generated
	StrategyName  string
}

// OrderIntent represents a validated order ready for execution.
type OrderIntent struct {
	ID              string
	ClientOrderID   string // Unique ID for idempotency
	Timestamp       time.Time
	Symbol          string
	Side            Side
	Contracts       int
	EntryPrice      decimal.Decimal // Limit price or expected fill
	StopLoss        decimal.Decimal
	TakeProfit      decimal.Decimal
	RiskAmount      decimal.Decimal // Actual $ at risk
	SignalID        string          // Reference to originating signal
	ExpiresAt       time.Time       // Order expiration
}

// OrderResult represents the result of an order execution.
type OrderResult struct {
	OrderID       string
	ClientOrderID string
	Status        OrderStatus
	FilledQty     int
	AvgFillPrice  decimal.Decimal
	Commission    decimal.Decimal
	Slippage      decimal.Decimal
	FilledAt      time.Time
	RejectReason  string
}

// Position represents an open position.
type Position struct {
	ID           string
	Symbol       string
	Side         Side
	Contracts    int
	EntryPrice   decimal.Decimal
	EntryTime    time.Time
	StopLoss     decimal.Decimal
	TakeProfit   decimal.Decimal
	UnrealizedPL decimal.Decimal
	RealizedPL   decimal.Decimal
}

// EquitySnapshot represents the account state at a point in time.
type EquitySnapshot struct {
	Timestamp     time.Time
	Equity        decimal.Decimal
	HighWaterMark decimal.Decimal
	Drawdown      decimal.Decimal // As ratio (0.15 = 15%)
	OpenPositions int
	DailyPL       decimal.Decimal
}

// Trade represents a completed trade (for audit trail).
type Trade struct {
	ID            string
	Symbol        string
	Side          Side
	Contracts     int
	EntryPrice    decimal.Decimal
	ExitPrice     decimal.Decimal
	EntryTime     time.Time
	ExitTime      time.Time
	GrossPL       decimal.Decimal
	Commission    decimal.Decimal
	NetPL         decimal.Decimal
	RMultiple     decimal.Decimal // Profit in terms of initial risk
	SignalID      string
	StrategyName  string
}

// InstrumentSpec defines the specifications of a trading instrument.
type InstrumentSpec struct {
	Symbol        string
	TickSize      decimal.Decimal // Minimum price movement
	TickValue     decimal.Decimal // Dollar value per tick per contract
	PointValue    decimal.Decimal // Dollar value per point
	MarginInitial decimal.Decimal
	MarginIntra   decimal.Decimal // Intraday margin
}

// Common instrument specifications.
var (
	InstrumentMES = InstrumentSpec{
		Symbol:        "MES",
		TickSize:      decimal.RequireFromString("0.25"),
		TickValue:     decimal.RequireFromString("1.25"),
		PointValue:    decimal.RequireFromString("5.00"),
		MarginInitial: decimal.RequireFromString("1500"),
		MarginIntra:   decimal.RequireFromString("50"),
	}

	InstrumentMGC = InstrumentSpec{
		Symbol:        "MGC",
		TickSize:      decimal.RequireFromString("0.10"),
		TickValue:     decimal.RequireFromString("1.00"),
		PointValue:    decimal.RequireFromString("10.00"),
		MarginInitial: decimal.RequireFromString("1100"),
		MarginIntra:   decimal.RequireFromString("550"),
	}
)

// GetInstrumentSpec returns the specification for a symbol.
func GetInstrumentSpec(symbol string) (InstrumentSpec, bool) {
	switch symbol {
	case "MES":
		return InstrumentMES, true
	case "MGC":
		return InstrumentMGC, true
	default:
		return InstrumentSpec{}, false
	}
}
