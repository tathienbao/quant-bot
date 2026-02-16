# Quant Trading Bot – MES / MGC – Risk-First Architecture

![Version](https://img.shields.io/badge/version-1.4.2-blue?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)
![Build](https://img.shields.io/github/actions/workflow/status/tathienbao/quant-bot/ci.yml?branch=main&style=flat-square)

## Quick Start

### Prerequisites

- Go 1.22+
- CGO enabled (for SQLite)

### Installation

```bash
git clone https://github.com/tathienbao/quant-bot.git
cd quant-bot
make build
```

### Usage

```bash
# Show version
./bin/quant-bot version

# Validate config file
./bin/quant-bot validate --config config.yaml

# Run backtest (interactive mode - recommended)
./bin/quant-bot backtest -i

# Run backtest (command line)
./bin/quant-bot backtest --config config.yaml --data data/MES_5m.csv --strategy grid

# Start bot (paper trading)
./bin/quant-bot run --config config.yaml --paper
```

### Commands

| Command | Description |
|---------|-------------|
| `version` | Show version, build time, git commit |
| `validate` | Validate configuration file |
| `backtest` | Run backtest with historical data |
| `run` | Start trading bot (paper/live) |
| `help` | Show usage information |

### Backtest Options

```bash
# Interactive mode (arrow keys to select strategy/data file)
./bin/quant-bot backtest -i

# Or specify options directly
./bin/quant-bot backtest \
  --config config.yaml \
  --data data/MES_5m.csv \
  --strategy grid \       # grid | grid-conservative | breakout | meanrev
  --verbose               # Enable debug logging
```

### Available Strategies

| Strategy | Return | Win Rate | Recommendation |
|----------|--------|----------|----------------|
| `grid` | +51.94% | 91.05% | ✅ Best performance |
| `grid-conservative` | +33.54% | 85.41% | ✅ Lower risk |
| `breakout` | -11.59% | 0% | ⚠️ Not recommended |
| `meanrev` | -3.62% | 20% | ⚠️ Not recommended |

*Results based on MES M5 data, $100k equity, 1% risk/trade, 2.5 months*

### Configuration

Copy `config.example.yaml` to `config.yaml` and adjust:

```yaml
account:
  starting_equity: 2000.0
  max_global_drawdown_pct: 0.20
  risk_per_trade_pct: 0.01

market:
  instrument_primary: "MES"
  timeframe: "5m"
```

### Testing

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Run fuzz tests
go test -fuzz=FuzzPositionSizer -fuzztime=30s ./internal/risk/
```

---

## 1. Project Objectives

Build a personal trading bot system following a **Quant** approach, with absolute priority on:

1. **Risk management and system robustness**, not short-term profit maximization.
2. **Clear layered architecture** (Observer → Strategy → Risk → Execution).
3. **Easy to backtest + easy to test**, scalable to multiple markets later.

### Target Markets (Phase 1–2)

- **Phase 1:** Micro E-mini S&P 500 futures (**MES**).
- **Phase 2:** Micro Gold futures (**MGC**).

Why micro futures:
- Sufficient volatility to generate many signals.
- Low margin requirements (MES from $50–300 intraday; MGC ~ $1,100).
- No funding rate like crypto.
- Costs and slippage are easy to model.

---

## 2. Initial Assumptions & Constraints

- Test capital: **$1,000–2,000**.
- Trade only **1 contract** MES initially (scale up later with logic).
- **No additional "artificial" leverage** beyond futures' inherent leverage.
- Focus on **intraday trading**: all positions closed before session end (avoid overnight margin jumps).
- Language: **Go** (primary), architecture and logic must remain consistent if ported to other languages.

---

## 3. Overall Architecture

Layered architecture:

```
┌─────────────┐
│   Observer  │ ← Market data, indicators
└──────┬──────┘
       │ MarketEvent
       ▼
┌─────────────┐
│  Strategy   │ ← Generate signals
└──────┬──────┘
       │ Signal
       ▼
┌─────────────┐
│ Risk Engine │ ← Position sizing, drawdown control (CORE)
└──────┬──────┘
       │ OrderIntent
       ▼
┌─────────────┐
│  Execution  │ ← Send orders (live/simulated)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ Persistence │ ← State recovery, audit trail
└─────────────┘
```

### 3.1. Observer (Data/Market Layer)

- **Purpose:** Fetch market data (ticks or 1m bars), calculate basic indicators (ATR/StdDev, moving average, etc.).
- **Output:** Standardized `MarketEvent` stream for upper layers.
- **Requirements:**
  - Pluggable `MarketDataFeed` abstraction: live feed, recorded feed (backtest).
  - No trading decision logic.
  - Runs in separate goroutine, pushes events via channel.

### 3.2. Strategy Layer

- **Purpose:** Receive `MarketEvent` → generate `Signal` (BUY, SELL, FLAT) + metadata (strength, reason).
- **Initially:**
  - Only need **1–2 extremely simple strategies** (e.g., breakout/mean-reversion) to test framework.
- **Requirements:**
  - Standard `Strategy` interface: `OnMarketEvent(ctx, event) []Signal`.
  - No sizing, no exposure handling – only says "should LONG/SHORT/EXIT".

### 3.3. Risk Engine (Project Core)

- **Purpose:** Transform `Signal` from Strategy into **risk-controlled `OrderIntent`**.
- **Main responsibilities:**
  - Track **equity**, **high-water mark**.
  - Calculate **global drawdown** and activate **Kill Switch** when threshold exceeded.
  - Calculate **position sizing** based on risk-per-trade and volatility (ATR/StdDev).
  - Control **exposure** (per symbol, total portfolio).
- This is the **most critical layer**, implemented and tested before everything else.
- **Single-threaded** to avoid race conditions on equity state.

### 3.4. Execution Layer

- **Purpose:** Receive `OrderIntent` → send actual order to broker or simulate fill in backtest.
- **Two modes:**
  - `LiveExecution`: calls broker API / trading platform.
  - `SimulatedExecution`: used in backtest.
- **Requirements:**
  - Ensure all orders go through Risk Engine first.
  - Full logging (timestamp, price, slippage, status).
  - Async with timeout, retry policy.
  - **Idempotent**: retries don't create duplicate orders.

### 3.5. Persistence Layer

- **Purpose:** Store state for post-crash recovery.
- **Stores:**
  - Equity snapshots (each fill).
  - Open positions.
  - Pending orders.
  - Trade history (audit trail).
- **Storage options:** SQLite (simple), PostgreSQL (production).

---

## 4. Concurrency Model

```go
// Observer chạy goroutine riêng, push qua channel
type MarketDataFeed interface {
    Subscribe(ctx context.Context, symbol string) (<-chan MarketEvent, error)
    Close() error
}

// Risk Engine xử lý tuần tự (single-threaded)
// Tất cả equity updates phải đi qua đây
type RiskEngine interface {
    ValidateAndSize(ctx context.Context, signal Signal) (*OrderIntent, error)
    UpdateEquity(snapshot EquitySnapshot)
    IsInSafeMode() bool
}

// Execution có thể async nhưng phải có timeout
type Executor interface {
    PlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error)
    CancelOrder(ctx context.Context, orderID string) error
}
```

**Rules:**
- Don't share mutable state between goroutines without synchronization.
- Use channels to communicate, not shared memory.
- Risk Engine state can only be modified from one goroutine.

---

## 5. Order State Machine

```
                    ┌──────────┐
                    │ CREATED  │
                    └────┬─────┘
                         │ submit
                         ▼
                    ┌──────────┐
          ┌─────────│ PENDING  │─────────┐
          │         └────┬─────┘         │
          │ reject       │ fill          │ cancel/expire
          ▼              ▼               ▼
    ┌──────────┐   ┌──────────┐    ┌──────────┐
    │ REJECTED │   │ PARTIAL  │    │ CANCELLED│
    └──────────┘   └────┬─────┘    └──────────┘
                        │ fill
                        ▼
                   ┌──────────┐
                   │  FILLED  │
                   └──────────┘
```

**Each transition must:**
- Be logged with timestamp
- Trigger equity update if filled
- Notify Risk Engine
- **Persist to storage** (for recovery)

---

## 6. System Invariants

The following invariants **must be strictly maintained** in all design and code:

### 6.1. Must Not Exceed Max Global Drawdown

- **Definition:** Global Drawdown = (HighWaterMarkEquity - CurrentEquity) / HighWaterMarkEquity.
- If `GlobalDD >= MaxAllowedDD` (e.g., 20%):
  - Automatically:
    - Close all positions.
    - Switch system to **SAFE MODE** (no new orders).
    - Log + **emit alert** (Telegram/Discord).

### 6.2. All Orders Must Go Through Risk Engine

- Strategy **must not** call Execution directly.
- Required flow: `Signal` → `RiskEngine.ValidateAndSize()` → `OrderIntent` → `Execution`.

### 6.3. Position Size Must Be Calculated Based on Risk-Per-Trade

- Don't use "fixed" position size like hardcoded 1 contract, except in initial testing.
- `PositionSize` must ensure: max loss if stopped out ≤ `risk_per_trade` (percentage of capital).

### 6.4. System Must Be Fail-Safe, Not Fail-Open

- If data connection lost, API down, or invalid input data, default behavior: **stop opening new orders**, may close/reduce positions if needed, log clearly.

### 6.5. Graceful Shutdown Required

- When receiving SIGINT/SIGTERM:
  - Stop accepting new signals from Strategy.
  - Wait for pending orders to confirm or timeout.
  - Depending on config: close positions or keep them.
  - **Persist state** before exit.

### 6.6. Order Idempotency

- Each order must have **unique client_order_id**.
- Retry with same client_order_id **must not** create duplicates.
- Check before submit: does order already exist?

### 6.7. Decimal Precision

- **DO NOT use float64** for money and prices in production.
- Use **fixed-point arithmetic** or `decimal` library.
- All calculations must be reproducible.

---

## 7. Configuration Parameters

```yaml
account:
  starting_equity: 1000.0
  max_global_drawdown_pct: 0.20  # 20%
  risk_per_trade_pct: 0.01       # 1% per trade

market:
  instrument_primary: "MES"
  instrument_secondary: "MGC"
  timeframe: "5m"
  timezone: "America/Chicago"    # CME timezone
  session_start: "17:00"         # Sunday 5pm CT
  session_end: "16:00"           # Friday 4pm CT
  daily_break_start: "16:00"     # Daily maintenance
  daily_break_end: "17:00"
  session_close_cutoff_min: 15   # Close orders X minutes before session close

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
  rate_limit_per_second: 10      # Broker API rate limit

health:
  heartbeat_interval_sec: 5
  max_missed_heartbeats: 3       # After 3 misses → SAFE MODE
  data_staleness_threshold_sec: 10

shutdown:
  timeout_sec: 30
  close_positions_on_shutdown: false

persistence:
  enabled: true
  type: "sqlite"                 # sqlite | postgres
  path: "./data/state.db"        # for sqlite
  # dsn: "postgres://..."        # for postgres
  snapshot_interval_sec: 60      # Periodic state snapshot

alerting:
  enabled: true
  channels:
    - type: "telegram"
      bot_token: "${TELEGRAM_BOT_TOKEN}"
      chat_id: "${TELEGRAM_CHAT_ID}"
    # - type: "discord"
    #   webhook_url: "${DISCORD_WEBHOOK}"
  events:
    - kill_switch_activated
    - connection_lost
    - daily_summary

metrics:
  enabled: true
  port: 9090                     # Prometheus metrics endpoint
  path: "/metrics"

backtest:
  slippage_ticks: 1
  commission_per_contract: 1.5   # USD round-trip
```

---

## 8. Alerting System

### Events requiring alerts:

| Event | Severity | Action |
|-------|----------|--------|
| Kill switch activated | CRITICAL | Immediate alert |
| Connection lost > 30s | HIGH | Alert + retry |
| Order rejected | MEDIUM | Log + alert if repeated |
| Daily P&L summary | INFO | End of day |
| System startup/shutdown | INFO | Confirmation |

### Channels supported:

- **Telegram** (recommended): Instant, free, easy setup
- **Discord**: Webhook integration
- **Email**: For daily summaries (optional)

---

## 9. Metrics & Observability

### Prometheus Metrics

```
# Counters
trading_orders_total{side="buy|sell", status="filled|rejected|cancelled"}
trading_signals_total{strategy="breakout|meanrev", direction="long|short"}

# Gauges
trading_equity_current
trading_drawdown_current
trading_positions_open{symbol="MES|MGC"}
trading_pnl_unrealized

# Histograms
trading_order_latency_seconds
trading_signal_to_fill_seconds
```

### Health Endpoints

```
GET /health          → {"status": "ok|degraded|unhealthy"}
GET /health/live     → Liveness probe
GET /health/ready    → Readiness probe (có data feed chưa?)
```

---

## 10. State Persistence & Recovery

### Data to persist:

1. **Equity snapshots** - each fill, each minute
2. **Open positions** - symbol, side, size, entry price, entry time
3. **Pending orders** - for reconciliation after restart
4. **High water mark** - for correct drawdown calculation
5. **Trade history** - audit trail

### Recovery flow:

```
Startup
  │
  ▼
Load last state from DB
  │
  ▼
Reconcile with broker
  ├── Position mismatch? → Alert + manual intervention
  └── Orders pending? → Check status, update
  │
  ▼
Resume normal operation
```

---

## 11. Rate Limiting

### Broker API limits (typical):

| Broker | Rate Limit |
|--------|------------|
| Interactive Brokers | 50 msg/sec |
| TradeStation | 10 req/sec |
| NinjaTrader | Varies |

### Implementation:

```go
type RateLimiter interface {
    Wait(ctx context.Context) error  // Block until allowed
    Allow() bool                      // Non-blocking check
}
```

- Dùng `golang.org/x/time/rate` (token bucket)
- Mỗi broker client có limiter riêng
- Graceful degradation khi hit limit

---

## 12. Technical Roadmap

### Phase 1 – Skeleton & Risk Engine

#### 1. Directory Structure (Go)

```
cmd/
  bot/                  # main entrypoint
internal/
  observer/             # MarketDataFeed, indicator calculation
  strategy/             # Strategy interface + simple strategies
  risk/                 # RiskEngine, PositionSizing, Drawdown control
  execution/            # LiveExecution, SimulatedExecution
  persistence/          # State storage, recovery
  alerting/             # Telegram, Discord notifications
  metrics/              # Prometheus metrics
  types/                # shared types
  config/               # config loading, validation
pkg/
  decimal/              # Fixed-point arithmetic
  indicator/            # ATR, SMA, StdDev
  ratelimit/            # Rate limiter utilities
```

#### 2. Implement Risk Engine First

- **HighWaterMarkTracker**
- **CheckGlobalDrawdown**
- **PositionSizer**
- **RiskEngine.ValidateOrder(ctx, signal)**

#### 3. Unit Tests

- Drawdown logic
- Position sizing
- Kill-switch

### Phase 2 – Observer & Strategy (MES only)

- Interface `MarketDataFeed` (live & backtest)
- ATR, moving average calculations
- 1–2 simple strategies

### Phase 3 – Execution & Backtest Loop

- SimulatedExecution
- Backtest runner
- Equity curve, metrics

### Phase 4 – Production Hardening

- Persistence layer
- Alerting integration
- Prometheus metrics
- Rate limiting
- Live broker integration

---

## 13. Development Commands

```bash
# Build
make build

# Run tests
make test

# Run with coverage
make test-coverage

# Lint
make lint

# Run bot (dev mode)
make run

# Run backtest
make backtest DATA=./data/mes_2024.csv

# Generate mocks
make mocks
```

---

## 14. Style & Quality Requirements

- Code must:
  - Be clear, prioritize **readability** over "tricky optimization".
  - Minimize unnecessary side-effects.
  - Log all important events thoroughly.
- **Unit tests are mandatory** for Risk Engine, Position Sizing, Drawdown logic.
- **No Machine Learning** in initial phase.
- **Don't "optimize backtest parameters"** – focus on **robustness**.
- **CI must pass** before merging.

---

## 15. Phase 1 Success Criteria

Backtest on MES (several months of data) with conditions:

- **Max Drawdown** < 10–15%.
- Positive expected profit after costs (commission + slippage).
- No "account blowup" bugs due to risk logic errors.
- Risk Engine & Execution work correctly as specified (proven through tests).
- Graceful shutdown works correctly.
- State recovery works correctly.
- Alerting works correctly.

**Only after achieving these, upgrade to MGC and/or live trading.**
