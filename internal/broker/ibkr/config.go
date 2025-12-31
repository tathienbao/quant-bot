// Package ibkr provides Interactive Brokers connectivity.
package ibkr

import (
	"time"
)

// Config holds IBKR connection configuration.
type Config struct {
	// Connection settings
	Host     string
	Port     int
	ClientID int

	// Timeouts
	ConnectTimeout time.Duration
	RequestTimeout time.Duration

	// Rate limiting
	MaxRequestsPerSecond int

	// Reconnection
	AutoReconnect     bool
	ReconnectInterval time.Duration
	MaxReconnectTries int

	// Paper trading
	PaperTrading bool
}

// DefaultConfig returns default IBKR configuration.
func DefaultConfig() Config {
	return Config{
		Host:                 "127.0.0.1",
		Port:                 7497, // Paper trading port
		ClientID:             1,
		ConnectTimeout:       10 * time.Second,
		RequestTimeout:       30 * time.Second,
		MaxRequestsPerSecond: 45, // IB limit is 50/sec
		AutoReconnect:        true,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectTries:    10,
		PaperTrading:         true,
	}
}

// LiveConfig returns configuration for live trading.
func LiveConfig() Config {
	cfg := DefaultConfig()
	cfg.Port = 7496 // Live trading port
	cfg.PaperTrading = false
	return cfg
}

// GatewayConfig returns configuration for IB Gateway.
func GatewayConfig(paper bool) Config {
	cfg := DefaultConfig()
	if paper {
		cfg.Port = 4002 // Gateway paper port
	} else {
		cfg.Port = 4001 // Gateway live port
	}
	cfg.PaperTrading = paper
	return cfg
}
