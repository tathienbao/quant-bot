package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
)

func TestLoadFromBytes_Valid(t *testing.T) {
	yaml := `
account:
  starting_equity: 1000.0
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01

market:
  instrument_primary: "MES"
  instrument_secondary: "MGC"
  timeframe: "5m"
  timezone: "America/Chicago"
  session_close_cutoff_min: 15

risk:
  volatility_lookback_bars: 20
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0

execution:
  order_timeout_sec: 5
  max_retries: 2
  retry_delay_ms: 500
  rate_limit_per_second: 10

persistence:
  enabled: false

backtest:
  slippage_ticks: 1
  commission_per_contract: 1.5
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if cfg.Account.StartingEquity != 1000.0 {
		t.Errorf("StartingEquity = %f, want 1000.0", cfg.Account.StartingEquity)
	}

	if cfg.Account.MaxGlobalDrawdownPct != 0.20 {
		t.Errorf("MaxGlobalDrawdownPct = %f, want 0.20", cfg.Account.MaxGlobalDrawdownPct)
	}

	if cfg.Market.InstrumentPrimary != "MES" {
		t.Errorf("InstrumentPrimary = %s, want MES", cfg.Market.InstrumentPrimary)
	}

	if cfg.Risk.StopLossATRMultiple != 2.0 {
		t.Errorf("StopLossATRMultiple = %f, want 2.0", cfg.Risk.StopLossATRMultiple)
	}
}

func TestLoadFromBytes_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "negative equity",
			yaml: `
account:
  starting_equity: -1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01
market:
  instrument_primary: "MES"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
`,
			wantErr: "starting_equity must be positive",
		},
		{
			name: "drawdown too high",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 1.5
  risk_per_trade_pct: 0.01
market:
  instrument_primary: "MES"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
`,
			wantErr: "max_global_drawdown_pct must be between 0 and 1",
		},
		{
			name: "risk too high",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.15
market:
  instrument_primary: "MES"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
`,
			wantErr: "risk_per_trade_pct must be between 0 and 0.1",
		},
		{
			name: "invalid instrument",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01
market:
  instrument_primary: "INVALID"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
`,
			wantErr: "is not supported",
		},
		{
			name: "missing instrument",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01
market:
  instrument_primary: ""
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
`,
			wantErr: "instrument_primary is required",
		},
		{
			name: "invalid persistence type",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01
market:
  instrument_primary: "MES"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
persistence:
  enabled: true
  type: "mysql"
`,
			wantErr: "persistence.type must be 'sqlite' or 'postgres'",
		},
		{
			name: "sqlite without path",
			yaml: `
account:
  starting_equity: 1000
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01
market:
  instrument_primary: "MES"
risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0
persistence:
  enabled: true
  type: "sqlite"
`,
			wantErr: "persistence.path is required for sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromBytes([]byte(tt.yaml))
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if tt.wantErr != "" && !contains(err.Error(), tt.wantErr) {
				t.Errorf("Error = %v, want to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ToRiskConfig(t *testing.T) {
	cfg := &Config{
		Account: AccountConfig{
			MaxGlobalDrawdownPct: 0.20,
			RiskPerTradePct:      0.01,
		},
		Risk: RiskConfig{
			MaxExposurePerSymbolPct: 0.5,
			MaxTotalExposurePct:     1.0,
			StopLossATRMultiple:     2.0,
			TakeProfitATRMultiple:   3.0,
		},
	}

	riskCfg := cfg.ToRiskConfig()

	if !riskCfg.MaxGlobalDrawdownPct.Equal(decimal.RequireFromString("0.2")) {
		t.Errorf("MaxGlobalDrawdownPct = %s, want 0.2", riskCfg.MaxGlobalDrawdownPct)
	}

	if !riskCfg.RiskPerTradePct.Equal(decimal.RequireFromString("0.01")) {
		t.Errorf("RiskPerTradePct = %s, want 0.01", riskCfg.RiskPerTradePct)
	}

	if !riskCfg.StopLossATRMultiple.Equal(decimal.RequireFromString("2")) {
		t.Errorf("StopLossATRMultiple = %s, want 2", riskCfg.StopLossATRMultiple)
	}
}

func TestConfig_Durations(t *testing.T) {
	cfg := &Config{
		Execution: ExecutionConfig{
			OrderTimeoutSec: 5,
			RetryDelayMs:    500,
		},
		Shutdown: ShutdownConfig{
			TimeoutSec: 30,
		},
		Persistence: PersistenceConfig{
			SnapshotIntervalSec: 60,
		},
	}

	if cfg.OrderTimeout().Seconds() != 5 {
		t.Errorf("OrderTimeout = %v, want 5s", cfg.OrderTimeout())
	}

	if cfg.RetryDelay().Milliseconds() != 500 {
		t.Errorf("RetryDelay = %v, want 500ms", cfg.RetryDelay())
	}

	if cfg.ShutdownTimeout().Seconds() != 30 {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout())
	}

	if cfg.SnapshotInterval().Seconds() != 60 {
		t.Errorf("SnapshotInterval = %v, want 60s", cfg.SnapshotInterval())
	}
}

func TestLoad_FromFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yaml := `
account:
  starting_equity: 2000.0
  max_global_drawdown_pct: 0.15
  risk_per_trade_pct: 0.02

market:
  instrument_primary: "MES"

risk:
  stop_loss_atr_multiple: 2.5
  take_profit_atr_multiple: 3.5
  max_exposure_per_symbol_pct: 0.4
  max_total_exposure_pct: 0.8
`

	if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Account.StartingEquity != 2000.0 {
		t.Errorf("StartingEquity = %f, want 2000.0", cfg.Account.StartingEquity)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoad_EnvironmentVariables(t *testing.T) {
	// Set environment variable
	os.Setenv("TEST_BOT_TOKEN", "my-secret-token")
	defer os.Unsetenv("TEST_BOT_TOKEN")

	yaml := `
account:
  starting_equity: 1000.0
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01

market:
  instrument_primary: "MES"

risk:
  stop_loss_atr_multiple: 2.0
  take_profit_atr_multiple: 3.0
  max_exposure_per_symbol_pct: 0.5
  max_total_exposure_pct: 1.0

alerting:
  enabled: true
  channels:
    - type: telegram
      bot_token: "${TEST_BOT_TOKEN}"
      chat_id: "12345"
`

	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Alerting.Channels) == 0 {
		t.Fatal("Expected alerting channels")
	}

	if cfg.Alerting.Channels[0].BotToken != "my-secret-token" {
		t.Errorf("BotToken = %s, want my-secret-token", cfg.Alerting.Channels[0].BotToken)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
