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

## [Unreleased]

### Planned for 0.6.0 (Phase 6: Alerting)
- [ ] Telegram alerting
- [ ] Event-based notifications
- [ ] Daily summary reports

### Planned for 1.0.0 (Production Ready)
- [ ] Live broker integration
- [ ] Persistence layer
- [ ] Alerting (Telegram)
- [ ] Prometheus metrics
- [ ] Full test coverage
