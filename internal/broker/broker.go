// Package broker provides broker connectivity for live trading.
package broker

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Common broker errors.
var (
	ErrNotConnected      = errors.New("broker not connected")
	ErrConnectionTimeout = errors.New("connection timeout")
	ErrOrderRejected     = errors.New("order rejected by broker")
	ErrInvalidContract   = errors.New("invalid contract")
	ErrRateLimited       = errors.New("rate limited by broker")
	ErrMarketClosed      = errors.New("market closed")
)

// ConnectionState represents the broker connection state.
type ConnectionState int

const (
	StateDisconnected ConnectionState = iota
	StateConnecting
	StateConnected
	StateError
)

func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Broker defines the interface for broker connectivity.
type Broker interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	State() ConnectionState
	IsConnected() bool

	// Account information
	GetAccountSummary(ctx context.Context) (*AccountSummary, error)

	// Market data
	SubscribeMarketData(ctx context.Context, symbol string) (<-chan types.MarketEvent, error)
	UnsubscribeMarketData(symbol string) error

	// Order execution
	PlaceOrder(ctx context.Context, order types.OrderIntent) (*OrderResult, error)
	CancelOrder(ctx context.Context, orderID string) error
	GetOpenOrders(ctx context.Context) ([]Order, error)

	// Position management
	GetPositions(ctx context.Context) ([]Position, error)
	GetPosition(ctx context.Context, symbol string) (*Position, error)

	// Graceful shutdown
	Shutdown(ctx context.Context) error
}

// AccountSummary contains account information.
type AccountSummary struct {
	AccountID        string
	Currency         string
	NetLiquidation   decimal.Decimal
	TotalCashValue   decimal.Decimal
	BuyingPower      decimal.Decimal
	AvailableFunds   decimal.Decimal
	ExcessLiquidity  decimal.Decimal
	InitMarginReq    decimal.Decimal
	MaintMarginReq   decimal.Decimal
	UnrealizedPnL    decimal.Decimal
	RealizedPnL      decimal.Decimal
	LastUpdated      time.Time
}

// Position represents a broker position.
type Position struct {
	Symbol        string
	Contracts     int
	Side          types.Side
	AvgCost       decimal.Decimal
	MarketPrice   decimal.Decimal
	MarketValue   decimal.Decimal
	UnrealizedPnL decimal.Decimal
	RealizedPnL   decimal.Decimal
	LastUpdated   time.Time
}

// Order represents a broker order.
type Order struct {
	OrderID       string
	ClientOrderID string
	Symbol        string
	Side          types.Side
	Quantity      int
	OrderType     OrderType
	LimitPrice    decimal.Decimal
	StopPrice     decimal.Decimal
	Status        OrderStatus
	FilledQty     int
	AvgFillPrice  decimal.Decimal
	Commission    decimal.Decimal
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// OrderType represents the type of order.
type OrderType string

const (
	OrderTypeMarket     OrderType = "MKT"
	OrderTypeLimit      OrderType = "LMT"
	OrderTypeStop       OrderType = "STP"
	OrderTypeStopLimit  OrderType = "STP LMT"
	OrderTypeTrailStop  OrderType = "TRAIL"
)

// OrderStatus represents the status of an order.
type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusSubmitted  OrderStatus = "submitted"
	OrderStatusFilled     OrderStatus = "filled"
	OrderStatusPartial    OrderStatus = "partial"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusRejected   OrderStatus = "rejected"
)

// OrderResult represents the result of placing an order.
type OrderResult struct {
	OrderID       string
	ClientOrderID string
	Status        OrderStatus
	Message       string
	SubmittedAt   time.Time
}

// Contract represents a tradeable contract.
type Contract struct {
	Symbol       string
	SecType      string // FUT, STK, OPT, etc.
	Exchange     string
	Currency     string
	LocalSymbol  string
	Multiplier   int
	Expiry       string // YYYYMM format for futures
}

// MESContract returns the MES futures contract specification.
func MESContract(expiry string) Contract {
	return Contract{
		Symbol:      "MES",
		SecType:     "FUT",
		Exchange:    "CME",
		Currency:    "USD",
		LocalSymbol: "MES" + expiry,
		Multiplier:  5,
		Expiry:      expiry,
	}
}

// MGCContract returns the MGC futures contract specification.
func MGCContract(expiry string) Contract {
	return Contract{
		Symbol:      "MGC",
		SecType:     "FUT",
		Exchange:    "COMEX",
		Currency:    "USD",
		LocalSymbol: "MGC" + expiry,
		Multiplier:  10,
		Expiry:      expiry,
	}
}

// GetFrontMonthExpiry returns the front month expiry in YYYYMM format.
// Futures typically expire on the 3rd Friday of Mar, Jun, Sep, Dec.
func GetFrontMonthExpiry(now time.Time) string {
	year := now.Year()
	month := now.Month()

	// Find the next quarterly expiry month
	quarterlyMonths := []time.Month{3, 6, 9, 12}
	for _, qm := range quarterlyMonths {
		if month <= qm {
			// Check if we're past the 3rd Friday of this month
			thirdFriday := getThirdFriday(year, qm)
			if now.Before(thirdFriday) {
				return formatExpiry(year, qm)
			}
		}
	}

	// Roll to next year's March
	return formatExpiry(year+1, 3)
}

func getThirdFriday(year int, month time.Month) time.Time {
	// Find the first day of the month
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)

	// Find the first Friday
	daysUntilFriday := (time.Friday - first.Weekday() + 7) % 7
	firstFriday := first.AddDate(0, 0, int(daysUntilFriday))

	// Third Friday is 14 days after first Friday
	return firstFriday.AddDate(0, 0, 14)
}

func formatExpiry(year int, month time.Month) string {
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).Format("200601")
}
