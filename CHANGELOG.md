# Changelog

Tất cả thay đổi đáng chú ý của dự án sẽ được ghi lại ở đây.

Format: [Semantic Versioning](https://semver.org/)
- **MAJOR** (Tự Hào): Thay đổi lớn, breaking changes, production-ready milestones
- **MINOR** (Tính Năng): Tính năng mới, backwards-compatible
- **PATCH** (Bug Fix): Sửa lỗi, improvements nhỏ

---

## [0.1.0] - 2024-12-31

### Phase 1: Skeleton & Risk Engine

#### Added
- **Core Types** (`internal/types/`)
  - `MarketEvent`, `Signal`, `OrderIntent`, `Position`, `EquitySnapshot`, `Trade`
  - `InstrumentSpec` cho MES và MGC với tick size, tick value, margin
  - Sentinel errors: `ErrKillSwitchActive`, `ErrExposureLimitExceeded`, etc.

- **Risk Engine** (`internal/risk/`)
  - `HighWaterMarkTracker`: Thread-safe equity peak tracking, drawdown calculation
  - `PositionSizer`: Risk-based position sizing với formula `contracts = floor(equity * risk% / (stopTicks * tickValue))`
  - `Engine`: Main risk engine với:
    - Kill switch tự động khi vượt max drawdown
    - SAFE MODE protection
    - Margin-based exposure limits (per-symbol và total)
    - Idempotent order ID generation
    - ATR-based stop loss calculation

- **Config** (`internal/config/`)
  - YAML config loader với environment variable expansion
  - Validation cho tất cả parameters
  - Conversion sang `risk.Config`

- **Project Infrastructure**
  - `go.mod` với dependencies (decimal, uuid, yaml)
  - `Makefile` với build, test, lint, coverage commands
  - `.golangci.yml` với 30+ linters
  - `.github/workflows/ci.yml` cho GitHub Actions
  - `.gitignore`, `config.example.yaml`, `.env.example`

#### Technical Details
- Language: Go 1.22+
- Test coverage: Risk Engine fully tested
- Thread-safety: All shared state protected với mutex
- Decimal precision: Sử dụng `shopspring/decimal` cho tiền/giá

---

## [0.2.0] - 2024-12-31

### Phase 2: Observer & Strategy

#### Added
- **Indicators** (`pkg/indicator/`)
  - `SMA`: Simple Moving Average with rolling window
  - `ATR`: Average True Range với True Range calculation
  - `StdDev`: Standard Deviation với Newton's method sqrt

- **Observer** (`internal/observer/`)
  - `MarketDataFeed` interface: Subscribe, Close, Name
  - `Calculator`: Combines ATR, SMA, StdDev calculations
  - `BacktestFeed`: CSV file reader with multiple timestamp formats
  - `MemoryFeed`: In-memory feed for testing

- **Strategies** (`internal/strategy/`)
  - `Strategy` interface: OnMarketEvent, Name, Reset
  - `SignalBuilder`: Fluent API for building signals
  - `Breakout`: Range breakout strategy với configurable lookback
  - `MeanReversion`: Bollinger-style mean reversion strategy
  - `MultiStrategy`: Combine multiple strategies

#### Technical Details
- Indicators use previous values for signal generation (no look-ahead bias)
- CSV parser supports: Unix timestamps, ISO 8601, common date formats
- All strategies prevent repeated signals for same condition

---

## [0.3.0] - 2024-12-31

### Phase 3: Execution & Backtest

#### Added
- **Execution** (`internal/execution/`)
  - `Executor` interface: PlaceOrder, CancelOrder, GetPosition, GetOpenOrders, Shutdown
  - `SimulatedExecutor`: Backtesting execution với:
    - Slippage simulation (configurable ticks)
    - Commission tracking
    - Stop loss / Take profit handling
    - Order idempotency via `usedOrderIDs` tracking
    - Position management (open/close)
    - Trade history recording

- **Backtest Runner** (`internal/backtest/`)
  - `Runner`: Main backtest engine với:
    - Market data feed integration
    - Strategy signal generation
    - Risk engine validation
    - Simulated execution
    - Equity curve tracking per bar
    - Time filtering (start/end time)
  - `Result`: Comprehensive backtest results
  - Time filtering support (StartTime, EndTime)

- **Performance Metrics** (`internal/backtest/metrics.go`)
  - `SharpeRatio`: Annualized risk-adjusted return
  - `SortinoRatio`: Downside risk-adjusted return
  - `CalmarRatio`: Return / Max Drawdown
  - `MaxDrawdown`: Maximum peak-to-trough decline
  - `WinRate`: Percentage of winning trades
  - `ProfitFactor`: Gross profit / Gross loss
  - `AverageWin`, `AverageLoss`: Mean trade P&L
  - `Expectancy`: Expected value per trade
  - `AnnualizedReturn`: Annualized total return

#### Technical Details
- SimulatedExecutor handles both opening and closing orders
- Equity curve records point per market event
- All metrics handle edge cases (no trades, empty curves)
- Decimal precision maintained throughout calculations

---

## [0.4.0] - 2024-12-31

### Phase 4: Integration

#### Added
- **CLI Interface** (`cmd/bot/main.go`)
  - `quant-bot version` - Show version, build time, git commit
  - `quant-bot help` - Show usage and available commands
  - `quant-bot validate --config <path>` - Validate config file
  - `quant-bot backtest --config <path> --data <csv> --strategy <name>` - Run backtest
  - `quant-bot run --config <path> --paper` - Start trading bot

- **Main Loop Integration**
  - Config loading from YAML with env var expansion
  - Risk engine initialization
  - Graceful shutdown with timeout
  - Step-by-step shutdown (stop loop → cancel orders → save state → close connections)

- **Build System Updates**
  - `make build` now injects version, build time, git commit via ldflags
  - Updated `make backtest` to use new CLI syntax

#### Technical Details
- CLI uses standard `flag` package (no external dependencies)
- Graceful shutdown handles SIGINT/SIGTERM
- JSON logging for `run` command, text logging for `backtest`
- Backtest command prints results and performance metrics

---

## [0.5.0] - 2024-12-31

### Phase 5: Persistence

#### Added
- **Persistence Package** (`internal/persistence/`)
  - `Repository` interface: Complete abstraction for state persistence
  - `SQLiteRepository`: Full SQLite implementation với:
    - Equity snapshots (save, get latest, get history)
    - Position management (save, get open, close)
    - Trade history (save, query by time, query by symbol)
    - Order tracking (save, get pending, update status)
    - Bot state (save/restore for recovery)
  - WAL mode enabled for better concurrent performance
  - Automatic migrations on startup

- **Recovery on Startup**
  - Bot state restored from SQLite on run
  - Logs recovered equity, high water mark, kill switch status
  - Graceful handling when no previous state exists

- **State Saving on Shutdown**
  - Current equity and high water mark saved
  - Kill switch / safe mode status preserved
  - Integrated into graceful shutdown sequence

- **Risk Engine Methods**
  - `CurrentEquity()` - Get current equity value
  - `HighWaterMark()` - Get peak equity value

#### Technical Details
- SQLite with `?_journal_mode=WAL&_busy_timeout=5000`
- All decimal values stored as strings for precision
- Comprehensive test coverage for all repository operations
- CGO required for go-sqlite3 driver

---

## [0.6.0] - 2024-12-31

### Phase 6: Alerting

#### Added
- **Alerting Package** (`internal/alerting/`)
  - `Alerter` interface: Standard interface for alert channels
  - `Severity` levels: Info, Warning, High, Critical with emoji support
  - `AlertEvent` types: Pre-defined events (kill switch, trades, bot lifecycle)
  - `TelegramAlerter`: Full Telegram Bot API integration
    - HTML formatting with emojis
    - Structured fields display
    - Daily summary reports
  - `ConsoleAlerter`: Log-based alerter for development
  - `MultiAlerter`: Fan-out to multiple channels concurrently
  - `MockAlerter`: Testing helper with assertion methods
  - `DailySummary`: Trading day statistics struct

- **Integration**
  - Alerter initialization from config
  - Bot started/stopped alerts
  - Graceful shutdown with alert notification

#### Technical Details
- Concurrent alert dispatch with error aggregation
- Thread-safe mock alerter for testing
- Config-driven channel selection
- Event-to-severity mapping

---

## [0.7.0] - 2024-12-31

### Phase 7: Metrics

#### Added
- **Metrics Package** (`internal/metrics/`)
  - `metrics.go` - Prometheus metrics definitions:
    - Trading: orders_total, trades_total, positions_open, position_contracts
    - Account: equity_current, equity_high_water_mark, drawdown_current, daily_pl, total_pl
    - Risk: safe_mode_active, signals_generated_total, signals_rejected_total
    - Latency: order_execution_seconds, data_feed_seconds, strategy_seconds
    - System: heartbeat_timestamp, data_feed_connected, broker_connected, uptime_seconds_total, errors_total
    - Build info metric with version, commit, build_time labels
  - `server.go` - HTTP server for metrics and health:
    - `/metrics` - Prometheus metrics endpoint
    - `/health` - Detailed health status with registered checks
    - `/ready` - Kubernetes readiness probe
    - `/live` - Kubernetes liveness probe
  - `recorder.go` - Helper methods for recording metrics
    - Timer utility for latency measurement

- **Integration**
  - Metrics server initialization in main loop
  - Health checks for risk engine and persistence
  - Build info metric set on startup
  - Graceful shutdown of metrics server

#### Technical Details
- Uses prometheus/client_golang for metrics
- promauto for automatic registration
- Health checks support healthy/unhealthy/degraded states
- Configurable port and paths via config.yaml

---

## [0.8.0] - 2024-12-31

### Phase 8: Broker Integration

#### Added
- **Broker Package** (`internal/broker/`)
  - `Broker` interface: Standard interface for broker connectivity
  - `AccountSummary`: Account information structure
  - `Position`, `Order`: Position and order tracking
  - `Contract`: Futures contract specification (MES, MGC)
  - `GetFrontMonthExpiry()`: Automatic front month calculation

- **IBKR Client** (`internal/broker/ibkr/`)
  - `Client`: TWS/IB Gateway connection
  - Socket-based IB API protocol implementation
  - Rate limiting (45 req/sec)
  - Auto-reconnect with configurable retry
  - Market data subscription
  - Order placement and cancellation
  - Position tracking

- **Paper Trading** (`internal/broker/paper/`)
  - `Broker`: Full simulated broker for testing
  - Slippage simulation
  - Commission tracking
  - Position P&L calculation
  - Order fill simulation with delay
  - No real connection required

- **Trading Engine** (`internal/engine/`)
  - `Engine`: Coordinates broker, risk, strategy
  - Main trading loop with market data processing
  - Signal generation and order execution
  - Equity update loop
  - Kill switch handling
  - Graceful shutdown

- **Configuration**
  - `BrokerConfig`: Broker connection settings
  - Support for IBKR and paper trading modes

#### Technical Details
- TWS ports: 7497 (paper), 7496 (live)
- Gateway ports: 4002 (paper), 4001 (live)
- Paper broker simulates realistic execution
- Full P&L tracking with realized/unrealized

#### Testing Flow
```
1. Paper Trading (internal simulation)
   └─> No connection needed, instant feedback

2. IBKR Paper Account (real-time simulation)
   └─> Real market data, simulated execution
   └─> Connect to TWS paper account (port 7497)

3. IBKR Live Trading (production)
   └─> Real money, real execution
   └─> Connect to TWS live account (port 7496)
```

---

## [1.3.0] - 2024-12-31

### Interactive CLI & Grid Strategy

#### Added
- **Grid/Rebound Strategy** (`internal/strategy/grid.go`)
  - Counter-trend strategy based on "raw math" approach
  - Two configurations:
    - `grid` (Original): +51.94% return, 91.05% win rate, 458 trades
    - `grid-conservative`: +33.54% return, 85.41% win rate, 233 trades
  - Parameters: GridSpacingPct, ReboundPct, MaxGridLevels, LookbackBars, StopLossPct
  - Optimized for M5/M15 timeframe on MES

- **Interactive CLI**
  - Arrow key navigation for strategy/data file selection
  - `-i` flag for interactive mode: `./bin/quant-bot backtest -i`
  - Visual display of strategy performance metrics
  - Auto-discovery of CSV files in `data/` directory

- **Strategy Documentation**
  - ASCII table comments at top of each strategy file
  - Backtest results, parameters, and CLI usage examples
  - Clear recommendations (✅ khuyến nghị / ⚠️ không khuyến nghị)

- **Sample Data**
  - `data/MES_5m.csv`: 13,208 bars (2.5 months M5 data)
  - `data/MES_1d.csv`: 253 bars (1 year daily data)

#### Changed
- Updated README with strategy comparison table
- Backtest command defaults to interactive selection when no flags provided

#### Technical Details
- Uses `github.com/manifoldco/promptui` for interactive CLI
- Grid strategy tracks swing high/low over lookback period
- Enters counter-trend at grid levels, exits on 10-20% rebound

---

## [1.0.0] - 2024-12-31

### Production Ready - Maximum Test Coverage

#### Added
- **Test Coverage Expansion** (~150 new tests)
  - Engine tests: 22 tests (Start, Stop, ProcessMarketEvent, KillSwitch, Failure injection)
  - IBKR mock tests: 21 tests (Connect, Disconnect, PlaceOrder - mocked)
  - Observer tests: 28 tests (Feed, Calculator, BacktestFeed, CSV parsing)
  - Risk engine: +18 tests (KS-01 to KS-06, SL-01, SL-02, Multi-instrument)
  - Types & Decimal tests: 9 tests (DEC-01 to DEC-03, InstrumentSpec)
  - Execution tests: +9 tests (Gap handling, P&L calculations)
  - Persistence tests: +5 tests (Recovery, state preservation)
  - Concurrency tests: 5 tests with `-race` flag
  - Fuzz tests: 5 fuzz tests for critical paths

- **README Quick Start Guide**
  - Installation instructions
  - CLI usage examples
  - Configuration guide
  - Testing commands

#### Technical Details
- All tests pass with `-race` flag
- Fuzz testing for position sizing, drawdown, OHLC validation
- Mock broker for IBKR testing without live connection
- Failure injection tests for resilience

---

## [Unreleased]

### Future Roadmap
- [ ] IBKR live integration tests (requires TWS)
- [ ] Live trading validation with paper account
- [ ] Contract rollover automation (ROLL-01 to ROLL-03)
