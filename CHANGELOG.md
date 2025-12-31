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

## [Unreleased]

### Planned for 0.2.0 (Phase 2: Observer & Strategy)
- [ ] MarketDataFeed interface
- [ ] Indicator calculations (ATR, SMA, StdDev)
- [ ] Backtest data feed
- [ ] Strategy interface
- [ ] Breakout strategy
- [ ] Mean Reversion strategy

### Planned for 0.3.0 (Phase 3: Execution & Backtest)
- [ ] SimulatedExecution
- [ ] Backtest runner
- [ ] Equity curve tracking
- [ ] Performance metrics

### Planned for 1.0.0 (Production Ready)
- [ ] Live broker integration
- [ ] Persistence layer
- [ ] Alerting (Telegram)
- [ ] Prometheus metrics
- [ ] Full test coverage
