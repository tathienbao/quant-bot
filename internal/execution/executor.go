// Package execution provides order execution functionality.
package execution

import (
	"context"

	"github.com/tathienbao/quant-bot/internal/types"
)

// Executor defines the interface for order execution.
type Executor interface {
	// PlaceOrder submits an order for execution.
	PlaceOrder(ctx context.Context, order types.OrderIntent) (*types.OrderResult, error)

	// CancelOrder cancels a pending order.
	CancelOrder(ctx context.Context, clientOrderID string) error

	// GetPosition returns the current position for a symbol.
	GetPosition(ctx context.Context, symbol string) (*types.Position, error)

	// GetOpenOrders returns all open orders.
	GetOpenOrders(ctx context.Context) ([]types.OrderIntent, error)

	// Shutdown gracefully shuts down the executor.
	Shutdown(ctx context.Context) error
}

// FillHandler is called when an order is filled.
type FillHandler func(ctx context.Context, result types.OrderResult)
