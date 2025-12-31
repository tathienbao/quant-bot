package types

import "errors"

// Sentinel errors for the trading system.
var (
	// Risk Engine errors
	ErrKillSwitchActive      = errors.New("kill switch active: system in safe mode")
	ErrExposureLimitExceeded = errors.New("exposure limit exceeded")
	ErrInsufficientEquity    = errors.New("insufficient equity for position size")
	ErrMaxDrawdownExceeded   = errors.New("maximum drawdown exceeded")

	// Order errors
	ErrDuplicateOrder   = errors.New("duplicate order id")
	ErrOrderTimeout     = errors.New("order timeout")
	ErrOrderRejected    = errors.New("order rejected by broker")
	ErrInvalidOrderSize = errors.New("invalid order size")

	// Data errors
	ErrInvalidPrice     = errors.New("invalid price value")
	ErrInvalidData      = errors.New("invalid market data")
	ErrStaleData        = errors.New("market data is stale")
	ErrDataUnavailable  = errors.New("market data unavailable")

	// Connection errors
	ErrConnectionLost    = errors.New("connection lost")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// State errors
	ErrPositionMismatch = errors.New("position mismatch with broker")
	ErrStateNotFound    = errors.New("state not found")

	// Validation errors
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrInvalidSymbol    = errors.New("invalid symbol")
	ErrInvalidTimeframe = errors.New("invalid timeframe")
)
