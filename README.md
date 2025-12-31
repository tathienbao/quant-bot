# Quant Trading Bot – MES / MGC – Risk-First Architecture

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

# Run backtest
./bin/quant-bot backtest --config config.yaml --data data/MES_5m.csv --strategy breakout

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
./bin/quant-bot backtest \
  --config config.yaml \
  --data data/MES_5m.csv \
  --strategy breakout \  # breakout | meanrev
  --verbose              # Enable debug logging
```

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

## 1. Mục tiêu dự án

Xây dựng một hệ thống trading bot cá nhân theo phong cách **Quant**, ưu tiên tuyệt đối cho:

1. **Quản trị rủi ro và độ bền hệ thống**, không phải tối đa hóa lợi nhuận ngắn hạn.
2. **Kiến trúc phân lớp rõ ràng** (Observer → Strategy → Risk → Execution).
3. **Dễ backtest + dễ kiểm thử**, có thể mở rộng lên nhiều thị trường sau này.

### Thị trường mục tiêu (giai đoạn 1–2)

- **Giai đoạn 1:** Micro E-mini S&P 500 futures (**MES**).
- **Giai đoạn 2:** Micro Gold futures (**MGC**).

Lý do chọn micro futures:
- Volatility đủ cao để tạo nhiều tín hiệu.
- Margin nhỏ (MES từ $50–300 intraday; MGC ~ $1,100).
- Không có funding rate như crypto.
- Chi phí và slippage dễ mô hình hóa.

---

## 2. Giả định & ràng buộc ban đầu

- Vốn test: **$1,000–2,000**.
- Chỉ trade **1 contract** MES trong giai đoạn đầu (sau này scale theo logic).
- **Không sử dụng đòn bẩy "ảo" thêm** ngoài leverage inherent của futures.
- Tập trung vào **intraday trading**: tất cả vị thế đóng trước khi kết thúc phiên (tránh overnight margin jump).
- Ngôn ngữ: **Go** (primary), kiến trúc và logic phải giữ nguyên nếu port sang ngôn ngữ khác.

---

## 3. Kiến trúc tổng thể

Kiến trúc phân lớp (layered architecture):

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

- **Nhiệm vụ:** Lấy dữ liệu market (ticks hoặc 1m bars), tính indicator cơ bản (ATR/StdDev, moving average, v.v.).
- **Output:** Stream `MarketEvent` chuẩn hóa cho các layer phía trên.
- **Yêu cầu:**
  - Có abstraction `MarketDataFeed` có thể plug: live feed, recorded feed (backtest).
  - Không chứa logic decision trading.
  - Chạy trong goroutine riêng, push event qua channel.

### 3.2. Strategy Layer

- **Nhiệm vụ:** Nhận `MarketEvent` → sinh `Signal` (BUY, SELL, FLAT) + meta (strength, reason).
- **Thời gian đầu:**
  - Chỉ cần **1–2 chiến lược cực đơn giản** (vd: breakout/mean-reversion) để test framework.
- **Yêu cầu:**
  - Interface `Strategy` chuẩn: `OnMarketEvent(ctx, event) []Signal`.
  - Không xử lý sizing, không xử lý exposure – chỉ nói "nên LONG/SHORT/EXIT".

### 3.3. Risk Engine (Core của dự án)

- **Nhiệm vụ:** Biến `Signal` từ Strategy thành **`OrderIntent` đã được kiểm soát rủi ro**.
- **Trách nhiệm chính:**
  - Theo dõi **equity**, **high-water mark**.
  - Tính **global drawdown** và kích hoạt **Kill Switch** khi vượt ngưỡng.
  - Tính **position sizing** dựa trên risk-per-trade và volatility (ATR/StdDev).
  - Kiểm soát **exposure** (per symbol, tổng danh mục).
- Đây là **layer quan trọng nhất**, được triển khai và test trước tất cả.
- **Single-threaded** để tránh race condition trên equity state.

### 3.4. Execution Layer

- **Nhiệm vụ:** Nhận `OrderIntent` → gửi lệnh thực tế tới broker hoặc simulate fill trong backtest.
- **Có 2 mode:**
  - `LiveExecution`: gọi API broker / trading platform.
  - `SimulatedExecution`: dùng trong backtest.
- **Yêu cầu:**
  - Đảm bảo tất cả lệnh đều đi qua Risk Engine trước.
  - Log đầy đủ (timestamp, price, slippage, trạng thái).
  - Async với timeout, có retry policy.
  - **Idempotent**: retry không tạo duplicate orders.

### 3.5. Persistence Layer

- **Nhiệm vụ:** Lưu trữ state để recovery sau crash.
- **Lưu trữ:**
  - Equity snapshots (mỗi fill).
  - Open positions.
  - Pending orders.
  - Trade history (audit trail).
- **Storage options:** SQLite (đơn giản), PostgreSQL (production).

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

**Quy tắc:**
- Không share mutable state giữa goroutines mà không có synchronization.
- Dùng channel để communicate, không dùng shared memory.
- Risk Engine state chỉ được modify từ một goroutine.

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

**Mỗi transition phải:**
- Được log với timestamp
- Trigger equity update nếu có fill
- Notify Risk Engine
- **Persist to storage** (cho recovery)

---

## 6. Invariants (Luật bất biến của hệ thống)

Các bất biến sau **phải được giữ chặt** trong mọi thiết kế và code:

### 6.1. Không được vượt Max Global Drawdown

- **Định nghĩa:** Global Drawdown = (HighWaterMarkEquity - CurrentEquity) / HighWaterMarkEquity.
- Nếu `GlobalDD >= MaxAllowedDD` (ví dụ 20%):
  - Tự động:
    - Đóng tất cả vị thế.
    - Chuyển hệ thống sang **SAFE MODE** (không mở lệnh mới).
    - Ghi log + **emit alert** (Telegram/Discord).

### 6.2. Mọi order phải đi qua Risk Engine

- Strategy **không được** gọi Execution trực tiếp.
- Bắt buộc: `Signal` → `RiskEngine.ValidateAndSize()` → `OrderIntent` → `Execution`.

### 6.3. Position size phải được tính dựa trên risk-per-trade

- Không dùng position size "cố định" kiểu 1 contract cứng, trừ giai đoạn test sơ khai.
- `PositionSize` phải đảm bảo: max loss nếu bị stop-out ≤ `risk_per_trade` (tỷ lệ phần trăm vốn).

### 6.4. Hệ thống phải fail-safe, không fail-open

- Nếu mất kết nối dữ liệu, API, hoặc dữ liệu đầu vào không hợp lệ, default hành vi: **dừng mở lệnh mới**, có thể đóng/giảm vị thế nếu cần, log rõ ràng.

### 6.5. Graceful Shutdown bắt buộc

- Khi nhận SIGINT/SIGTERM:
  - Dừng nhận signal mới từ Strategy.
  - Đợi pending orders confirm hoặc timeout.
  - Tùy config: đóng vị thế hoặc giữ nguyên.
  - **Persist state** trước khi exit.

### 6.6. Order Idempotency

- Mỗi order phải có **unique client_order_id**.
- Retry với cùng client_order_id **không được** tạo duplicate.
- Check trước khi submit: order đã tồn tại chưa?

### 6.7. Decimal Precision

- **KHÔNG dùng float64** cho tiền và giá trong production.
- Dùng **fixed-point arithmetic** hoặc `decimal` library.
- Tất cả calculations phải reproducible.

---

## 7. Thông số cấu hình (config)

```yaml
account:
  starting_equity: 1000.0
  max_global_drawdown_pct: 0.20  # 20%
  risk_per_trade_pct: 0.01       # 1% mỗi lệnh

market:
  instrument_primary: "MES"
  instrument_secondary: "MGC"
  timeframe: "5m"
  timezone: "America/Chicago"    # CME timezone
  session_start: "17:00"         # Sunday 5pm CT
  session_end: "16:00"           # Friday 4pm CT
  daily_break_start: "16:00"     # Daily maintenance
  daily_break_end: "17:00"
  session_close_cutoff_min: 15   # Đóng lệnh trước close X phút

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
  max_missed_heartbeats: 3       # Sau 3 lần miss → SAFE MODE
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

### Events cần alert:

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

### Dữ liệu cần persist:

1. **Equity snapshots** - mỗi fill, mỗi phút
2. **Open positions** - symbol, side, size, entry price, entry time
3. **Pending orders** - để reconcile sau restart
4. **High water mark** - để tính drawdown đúng
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

## 12. Roadmap kỹ thuật

### Phase 1 – Skeleton & Risk Engine

#### 1. Cấu trúc thư mục (Go)

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

#### 2. Implement Risk Engine trước

- **HighWaterMarkTracker**
- **CheckGlobalDrawdown**
- **PositionSizer**
- **RiskEngine.ValidateOrder(ctx, signal)**

#### 3. Unit tests

- Drawdown logic
- Position sizing
- Kill-switch

### Phase 2 – Observer & Strategy (MES only)

- Interface `MarketDataFeed` (live & backtest)
- ATR, moving average calculations
- 1–2 simple strategies

### Phase 3 – Execution & Backtest loop

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

## 14. Yêu cầu về Style & Quality

- Code phải:
  - Rõ ràng, ưu tiên **tính dễ đọc** hơn "tricky optimization".
  - Hạn chế side-effect không cần thiết.
  - Log đầy đủ những event quan trọng.
- **Unit test là bắt buộc** cho Risk Engine, Position Sizing, Drawdown logic.
- **Không đưa Machine Learning** vào giai đoạn đầu.
- **Không cố "tối ưu tham số backtest"** – focus vào **robustness**.
- **CI phải pass** trước khi merge.

---

## 15. Mục tiêu thành công giai đoạn 1

Backtest trên MES (dữ liệu vài tháng) với điều kiện:

- **Max Drawdown** < 10–15%.
- Lợi nhuận kỳ vọng dương sau chi phí (commission + slippage).
- Không có bug kiểu "cháy tài khoản" do lỗi logic risk.
- Risk Engine & Execution hoạt động đúng như spec (được chứng minh qua test).
- Graceful shutdown hoạt động đúng.
- State recovery hoạt động đúng.
- Alerting hoạt động đúng.

**Khi đạt được, mới nâng cấp sang MGC và/hoặc live trading.**
