// Package persistence provides state persistence functionality.
package persistence

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// Repository defines the interface for state persistence.
type Repository interface {
	// Equity operations
	SaveEquitySnapshot(ctx context.Context, snapshot EquitySnapshot) error
	GetLatestEquitySnapshot(ctx context.Context) (*EquitySnapshot, error)
	GetEquityHistory(ctx context.Context, from, to time.Time) ([]EquitySnapshot, error)

	// Position operations
	SavePosition(ctx context.Context, position types.Position) error
	GetOpenPositions(ctx context.Context) ([]types.Position, error)
	ClosePosition(ctx context.Context, positionID string, exitPrice decimal.Decimal, exitTime time.Time) error

	// Trade operations
	SaveTrade(ctx context.Context, trade types.Trade) error
	GetTrades(ctx context.Context, from, to time.Time) ([]types.Trade, error)
	GetTradesBySymbol(ctx context.Context, symbol string, limit int) ([]types.Trade, error)

	// Order operations
	SaveOrder(ctx context.Context, order OrderRecord) error
	GetPendingOrders(ctx context.Context) ([]OrderRecord, error)
	UpdateOrderStatus(ctx context.Context, clientOrderID string, status types.OrderStatus, fillPrice decimal.Decimal) error

	// State operations
	SaveState(ctx context.Context, state BotState) error
	GetState(ctx context.Context) (*BotState, error)

	// Lifecycle
	Close() error
	Migrate(ctx context.Context) error
}

// EquitySnapshot represents persisted equity state.
type EquitySnapshot struct {
	ID            int64
	Timestamp     time.Time
	Equity        decimal.Decimal
	HighWaterMark decimal.Decimal
	Drawdown      decimal.Decimal
	OpenPositions int
	DailyPL       decimal.Decimal
}

// OrderRecord represents a persisted order.
type OrderRecord struct {
	ID              int64
	ClientOrderID   string
	Symbol          string
	Side            types.Side
	Contracts       int
	EntryPrice      decimal.Decimal
	StopLoss        decimal.Decimal
	TakeProfit      decimal.Decimal
	Status          types.OrderStatus
	FilledPrice     decimal.Decimal
	FilledAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	SignalID        string
	StrategyName    string
}

// BotState represents the overall bot state for recovery.
type BotState struct {
	ID              int64
	LastUpdated     time.Time
	Equity          decimal.Decimal
	HighWaterMark   decimal.Decimal
	KillSwitchActive bool
	SafeModeActive  bool
	TotalTrades     int
	WinningTrades   int
	LosingTrades    int
	TotalPL         decimal.Decimal
}
