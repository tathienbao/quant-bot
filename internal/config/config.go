// Package config handles configuration loading and validation.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/types"
	"gopkg.in/yaml.v3"
)

// Config represents the full application configuration.
type Config struct {
	Account     AccountConfig     `yaml:"account"`
	Market      MarketConfig      `yaml:"market"`
	Risk        RiskConfig        `yaml:"risk"`
	Execution   ExecutionConfig   `yaml:"execution"`
	Health      HealthConfig      `yaml:"health"`
	Shutdown    ShutdownConfig    `yaml:"shutdown"`
	Persistence PersistenceConfig `yaml:"persistence"`
	Alerting    AlertingConfig    `yaml:"alerting"`
	Metrics     MetricsConfig     `yaml:"metrics"`
	Backtest    BacktestConfig    `yaml:"backtest"`
	Broker      BrokerConfig      `yaml:"broker"`
}

// AccountConfig holds account-related settings.
type AccountConfig struct {
	StartingEquity       float64 `yaml:"starting_equity"`
	MaxGlobalDrawdownPct float64 `yaml:"max_global_drawdown_pct"`
	RiskPerTradePct      float64 `yaml:"risk_per_trade_pct"`
}

// MarketConfig holds market-related settings.
type MarketConfig struct {
	InstrumentPrimary     string `yaml:"instrument_primary"`
	InstrumentSecondary   string `yaml:"instrument_secondary"`
	Timeframe             string `yaml:"timeframe"`
	Timezone              string `yaml:"timezone"`
	SessionStart          string `yaml:"session_start"`
	SessionEnd            string `yaml:"session_end"`
	DailyBreakStart       string `yaml:"daily_break_start"`
	DailyBreakEnd         string `yaml:"daily_break_end"`
	SessionCloseCutoffMin int    `yaml:"session_close_cutoff_min"`
}

// RiskConfig holds risk management settings.
type RiskConfig struct {
	VolatilityLookbackBars  int     `yaml:"volatility_lookback_bars"`
	StopLossATRMultiple     float64 `yaml:"stop_loss_atr_multiple"`
	TakeProfitATRMultiple   float64 `yaml:"take_profit_atr_multiple"`
	MaxExposurePerSymbolPct float64 `yaml:"max_exposure_per_symbol_pct"`
	MaxTotalExposurePct     float64 `yaml:"max_total_exposure_pct"`
}

// ExecutionConfig holds execution settings.
type ExecutionConfig struct {
	OrderTimeoutSec     int `yaml:"order_timeout_sec"`
	MaxRetries          int `yaml:"max_retries"`
	RetryDelayMs        int `yaml:"retry_delay_ms"`
	RateLimitPerSecond  int `yaml:"rate_limit_per_second"`
}

// HealthConfig holds health check settings.
type HealthConfig struct {
	HeartbeatIntervalSec      int `yaml:"heartbeat_interval_sec"`
	MaxMissedHeartbeats       int `yaml:"max_missed_heartbeats"`
	DataStalenessThresholdSec int `yaml:"data_staleness_threshold_sec"`
}

// ShutdownConfig holds shutdown settings.
type ShutdownConfig struct {
	TimeoutSec              int  `yaml:"timeout_sec"`
	ClosePositionsOnShutdown bool `yaml:"close_positions_on_shutdown"`
}

// PersistenceConfig holds persistence settings.
type PersistenceConfig struct {
	Enabled             bool   `yaml:"enabled"`
	Type                string `yaml:"type"` // sqlite | postgres
	Path                string `yaml:"path"` // for sqlite
	DSN                 string `yaml:"dsn"`  // for postgres
	SnapshotIntervalSec int    `yaml:"snapshot_interval_sec"`
}

// AlertingConfig holds alerting settings.
type AlertingConfig struct {
	Enabled  bool            `yaml:"enabled"`
	Channels []ChannelConfig `yaml:"channels"`
	Events   []string        `yaml:"events"`
}

// ChannelConfig holds a single alert channel configuration.
type ChannelConfig struct {
	Type       string `yaml:"type"` // telegram | discord | email
	BotToken   string `yaml:"bot_token"`
	ChatID     string `yaml:"chat_id"`
	WebhookURL string `yaml:"webhook_url"`
}

// MetricsConfig holds metrics settings.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// BacktestConfig holds backtest settings.
type BacktestConfig struct {
	SlippageTicks         int     `yaml:"slippage_ticks"`
	CommissionPerContract float64 `yaml:"commission_per_contract"`
}

// BrokerConfig holds broker settings.
type BrokerConfig struct {
	Type     string `yaml:"type"`      // ibkr, paper
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	ClientID int    `yaml:"client_id"`
}

// Load loads configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// LoadFromBytes loads configuration from YAML bytes.
func LoadFromBytes(data []byte) (*Config, error) {
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	var errs []string

	// Account validation
	if c.Account.StartingEquity <= 0 {
		errs = append(errs, "account.starting_equity must be positive")
	}
	if c.Account.MaxGlobalDrawdownPct <= 0 || c.Account.MaxGlobalDrawdownPct > 1 {
		errs = append(errs, "account.max_global_drawdown_pct must be between 0 and 1")
	}
	if c.Account.RiskPerTradePct <= 0 || c.Account.RiskPerTradePct > 0.1 {
		errs = append(errs, "account.risk_per_trade_pct must be between 0 and 0.1 (10%)")
	}

	// Market validation
	if c.Market.InstrumentPrimary == "" {
		errs = append(errs, "market.instrument_primary is required")
	}
	if _, ok := types.GetInstrumentSpec(c.Market.InstrumentPrimary); !ok && c.Market.InstrumentPrimary != "" {
		errs = append(errs, fmt.Sprintf("market.instrument_primary '%s' is not supported", c.Market.InstrumentPrimary))
	}

	// Risk validation
	if c.Risk.StopLossATRMultiple <= 0 {
		errs = append(errs, "risk.stop_loss_atr_multiple must be positive")
	}
	if c.Risk.TakeProfitATRMultiple <= 0 {
		errs = append(errs, "risk.take_profit_atr_multiple must be positive")
	}
	if c.Risk.MaxExposurePerSymbolPct <= 0 || c.Risk.MaxExposurePerSymbolPct > 1 {
		errs = append(errs, "risk.max_exposure_per_symbol_pct must be between 0 and 1")
	}
	if c.Risk.MaxTotalExposurePct <= 0 || c.Risk.MaxTotalExposurePct > 2 {
		errs = append(errs, "risk.max_total_exposure_pct must be between 0 and 2")
	}

	// Execution validation
	if c.Execution.OrderTimeoutSec <= 0 {
		c.Execution.OrderTimeoutSec = 5 // default
	}
	if c.Execution.MaxRetries < 0 {
		c.Execution.MaxRetries = 2 // default
	}

	// Persistence validation
	if c.Persistence.Enabled {
		if c.Persistence.Type != "sqlite" && c.Persistence.Type != "postgres" {
			errs = append(errs, "persistence.type must be 'sqlite' or 'postgres'")
		}
		if c.Persistence.Type == "sqlite" && c.Persistence.Path == "" {
			errs = append(errs, "persistence.path is required for sqlite")
		}
		if c.Persistence.Type == "postgres" && c.Persistence.DSN == "" {
			errs = append(errs, "persistence.dsn is required for postgres")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %s", types.ErrInvalidConfig, strings.Join(errs, "; "))
	}

	return nil
}

// ToRiskConfig converts to risk.Config.
func (c *Config) ToRiskConfig() risk.Config {
	return risk.Config{
		MaxGlobalDrawdownPct:    decimal.NewFromFloat(c.Account.MaxGlobalDrawdownPct),
		RiskPerTradePct:         decimal.NewFromFloat(c.Account.RiskPerTradePct),
		MaxExposurePerSymbolPct: decimal.NewFromFloat(c.Risk.MaxExposurePerSymbolPct),
		MaxTotalExposurePct:     decimal.NewFromFloat(c.Risk.MaxTotalExposurePct),
		StopLossATRMultiple:     decimal.NewFromFloat(c.Risk.StopLossATRMultiple),
		TakeProfitATRMultiple:   decimal.NewFromFloat(c.Risk.TakeProfitATRMultiple),
	}
}

// StartingEquityDecimal returns starting equity as decimal.
func (c *Config) StartingEquityDecimal() decimal.Decimal {
	return decimal.NewFromFloat(c.Account.StartingEquity)
}

// OrderTimeout returns the order timeout duration.
func (c *Config) OrderTimeout() time.Duration {
	return time.Duration(c.Execution.OrderTimeoutSec) * time.Second
}

// RetryDelay returns the retry delay duration.
func (c *Config) RetryDelay() time.Duration {
	return time.Duration(c.Execution.RetryDelayMs) * time.Millisecond
}

// ShutdownTimeout returns the shutdown timeout duration.
func (c *Config) ShutdownTimeout() time.Duration {
	return time.Duration(c.Shutdown.TimeoutSec) * time.Second
}

// SnapshotInterval returns the snapshot interval duration.
func (c *Config) SnapshotInterval() time.Duration {
	return time.Duration(c.Persistence.SnapshotIntervalSec) * time.Second
}

// IsAlertEventEnabled checks if an alert event type is enabled.
func (c *Config) IsAlertEventEnabled(event string) bool {
	if !c.Alerting.Enabled {
		return false
	}
	// If no events specified, all are enabled
	if len(c.Alerting.Events) == 0 {
		return true
	}
	for _, e := range c.Alerting.Events {
		if e == event || e == "all" {
			return true
		}
	}
	return false
}
