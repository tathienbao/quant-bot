# Claude Guidelines – Quant Bot (MES / MGC)

File này định nghĩa **mindset** và **phong cách code** mà Claude cần tuân thủ khi làm việc trên dự án quant trading bot này.

---

## 1. Mindset tổng quát

1. **Risk-first, không phải profit-first**
   - Ưu tiên hàng đầu là **không cháy tài khoản**, không phải tìm "chén thánh".
   - Mọi thiết kế đều phải trả lời: "Nếu mọi thứ đi sai cùng lúc, hệ thống sẽ bảo vệ vốn như thế nào?"

2. **Kỹ sư hệ thống, không phải "trader chém gió"**
   - Tránh giải thích lan man về trading, indicator, "bí kíp".
   - Tập trung vào:
     - Kiến trúc rõ ràng.
     - Contract giữa các module.
     - Đảm bảo code dễ test và dễ debug.

3. **Simplicity > Cleverness**
   - Giải pháp đơn giản, dễ kiểm chứng được ưu tiên hơn code "thông minh" nhưng khó đọc.
   - Nếu phải lựa chọn: **đọc hiểu nhanh** quan trọng hơn vài phần trăm hiệu năng.

4. **Deterministic & testable**
   - Mỗi module phải có input/output rõ ràng.
   - Không viết logic critical mà không có unit test.

5. **Không over-optimise tham số**
   - Không cố vẽ lại quá khứ bằng cách tuning tham số đến mức hoàn hảo.
   - Tập trung vào **robustness**: chiến lược vẫn ổn khi tham số thay đổi trong một khoảng.

---

## 2. Go Idioms & Patterns

### 2.1. Error Handling

```go
// ✅ Wrap error với context
if err != nil {
    return fmt.Errorf("validate order %s: %w", order.ID, err)
}

// ✅ Sentinel errors cho business logic
var (
    ErrKillSwitchActive      = errors.New("kill switch active")
    ErrExposureLimitExceeded = errors.New("exposure limit exceeded")
    ErrInsufficientEquity    = errors.New("insufficient equity for position size")
    ErrDuplicateOrder        = errors.New("duplicate order id")
    ErrInvalidPrice          = errors.New("invalid price value")
)

// ✅ Check sentinel errors
if errors.Is(err, ErrKillSwitchActive) {
    // handle specifically
}

// ❌ KHÔNG swallow error
if err != nil {
    log.Println(err)  // rồi tiếp tục như không có gì
}

// ❌ KHÔNG return error string không có context
return errors.New("failed")
```

### 2.2. context.Context

```go
// ✅ Mọi hàm có thể cancel/timeout PHẢI nhận context là param đầu tiên
func (e *Executor) PlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error)
func (f *Feed) Subscribe(ctx context.Context, symbol string) (<-chan MarketEvent, error)

// ✅ Respect context cancellation
select {
case <-ctx.Done():
    return nil, ctx.Err()
case event := <-eventCh:
    // process
}

// ✅ Timeout cho external calls
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()
result, err := broker.PlaceOrder(ctx, order)

// ❌ KHÔNG ignore context
func (e *Executor) PlaceOrder(order OrderIntent) (*OrderResult, error)  // thiếu ctx
```

### 2.3. Interface Design

```go
// ✅ Interface nhỏ, định nghĩa ở nơi SỬ DỤNG (consumer)
// File: internal/risk/engine.go
type EquityProvider interface {
    CurrentEquity() Decimal
}

type PositionReader interface {
    OpenPositions(symbol string) []Position
}

// ✅ Accept interfaces, return structs
func NewRiskEngine(equity EquityProvider, cfg Config) *RiskEngine

// ❌ KHÔNG tạo God interface
type TradingSystem interface {
    GetEquity() float64
    GetPositions() []Position
    PlaceOrder(order Order) error
    // ... 20 methods khác
}
```

### 2.4. Dependency Injection

```go
// ✅ Inject dependencies qua constructor
func NewRiskEngine(
    equityProvider EquityProvider,
    config RiskConfig,
    logger *slog.Logger,
    alerter Alerter,
) *RiskEngine {
    return &RiskEngine{
        equity:  equityProvider,
        cfg:     config,
        log:     logger,
        alerter: alerter,
    }
}

// ❌ KHÔNG dùng global state
var globalEquity float64  // Race condition waiting to happen

// ❌ KHÔNG init dependencies trong constructor
func NewRiskEngine() *RiskEngine {
    return &RiskEngine{
        db: connectToDatabase(),  // Hard to test
    }
}
```

### 2.5. Concurrency

```go
// ✅ Communicate qua channels
func (o *Observer) Start(ctx context.Context) <-chan MarketEvent {
    ch := make(chan MarketEvent, 100)
    go func() {
        defer close(ch)
        for {
            select {
            case <-ctx.Done():
                return
            default:
                event := o.fetchNext()
                ch <- event
            }
        }
    }()
    return ch
}

// ✅ Mutex cho shared state đơn giản
type HighWaterMark struct {
    mu   sync.RWMutex
    peak Decimal
}

func (h *HighWaterMark) Update(equity Decimal) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if equity.GreaterThan(h.peak) {
        h.peak = equity
    }
}

// ❌ KHÔNG share mutable state không có sync
```

---

## 3. Decimal Precision (Critical)

```go
// ✅ Dùng shopspring/decimal cho tiền và giá
import "github.com/shopspring/decimal"

type Money = decimal.Decimal
type Price = decimal.Decimal

// ✅ Tạo decimal từ string (safe)
price := decimal.RequireFromString("1234.50")

// ✅ Arithmetic
total := price.Mul(decimal.NewFromInt(int64(quantity)))
pnl := exitPrice.Sub(entryPrice).Mul(decimal.NewFromInt(int64(contracts)))

// ✅ Comparison
if equity.LessThan(minEquity) {
    return ErrInsufficientEquity
}

// ❌ KHÔNG dùng float64 cho tiền
var equity float64 = 1000.0  // Precision issues!
result := 0.1 + 0.2          // = 0.30000000000000004

// ❌ KHÔNG parse float rồi convert
f, _ := strconv.ParseFloat("1234.50", 64)
price := decimal.NewFromFloat(f)  // Already lost precision!
```

### Decimal trong Types

```go
type Position struct {
    Symbol      string
    Side        Side
    Contracts   int
    EntryPrice  decimal.Decimal
    CurrentPnL  decimal.Decimal
}

type EquitySnapshot struct {
    Timestamp   time.Time
    Equity      decimal.Decimal
    HighWater   decimal.Decimal
    Drawdown    decimal.Decimal  // As ratio, e.g., 0.15 = 15%
}
```

---

## 4. Order Idempotency

```go
// ✅ Generate unique client order ID
func GenerateOrderID() string {
    return fmt.Sprintf("%s-%s",
        time.Now().Format("20060102-150405"),
        uuid.New().String()[:8],
    )
}

// ✅ Check duplicate before submit
type OrderTracker struct {
    mu     sync.RWMutex
    orders map[string]*Order  // clientOrderID -> Order
}

func (t *OrderTracker) Submit(ctx context.Context, order *Order) error {
    t.mu.Lock()
    defer t.mu.Unlock()

    if _, exists := t.orders[order.ClientOrderID]; exists {
        return ErrDuplicateOrder
    }

    t.orders[order.ClientOrderID] = order
    return t.executor.PlaceOrder(ctx, order)
}

// ✅ Retry với same ID = idempotent
func (e *Executor) PlaceOrderWithRetry(ctx context.Context, order OrderIntent) (*OrderResult, error) {
    var lastErr error
    for attempt := 0; attempt < e.maxRetries; attempt++ {
        result, err := e.PlaceOrder(ctx, order)  // Same order.ClientOrderID
        if err == nil {
            return result, nil
        }
        lastErr = err

        // Exponential backoff
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(e.retryDelay * time.Duration(1<<attempt)):
        }
    }
    return nil, fmt.Errorf("after %d retries: %w", e.maxRetries, lastErr)
}
```

---

## 5. Persistence Pattern

```go
// ✅ Repository interface
type StateRepository interface {
    SaveEquitySnapshot(ctx context.Context, snapshot EquitySnapshot) error
    GetLatestEquity(ctx context.Context) (*EquitySnapshot, error)

    SavePosition(ctx context.Context, position Position) error
    GetOpenPositions(ctx context.Context) ([]Position, error)

    SaveOrder(ctx context.Context, order Order) error
    GetPendingOrders(ctx context.Context) ([]Order, error)

    SaveTrade(ctx context.Context, trade Trade) error  // Audit trail
}

// ✅ Transaction wrapper
func (r *SQLiteRepo) WithTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }

    if err := fn(tx); err != nil {
        if rbErr := tx.Rollback(); rbErr != nil {
            return fmt.Errorf("rollback failed: %v (original: %w)", rbErr, err)
        }
        return err
    }

    return tx.Commit()
}

// ✅ Recovery on startup
func (e *Engine) RecoverState(ctx context.Context) error {
    snapshot, err := e.repo.GetLatestEquity(ctx)
    if err != nil {
        return fmt.Errorf("load equity: %w", err)
    }
    e.equity = snapshot.Equity
    e.highWater = snapshot.HighWater

    positions, err := e.repo.GetOpenPositions(ctx)
    if err != nil {
        return fmt.Errorf("load positions: %w", err)
    }
    e.positions = positions

    // Reconcile with broker
    brokerPositions, err := e.broker.GetPositions(ctx)
    if err != nil {
        return fmt.Errorf("get broker positions: %w", err)
    }

    if !positionsMatch(positions, brokerPositions) {
        e.alerter.Alert(ctx, AlertCritical, "Position mismatch detected!")
        return ErrPositionMismatch
    }

    return nil
}
```

---

## 6. Alerting Pattern

```go
// ✅ Alerter interface
type Alerter interface {
    Alert(ctx context.Context, severity Severity, message string, fields ...any) error
}

type Severity int

const (
    AlertInfo Severity = iota
    AlertWarning
    AlertHigh
    AlertCritical
)

// ✅ Multi-channel alerter
type MultiAlerter struct {
    channels []Alerter
}

func (m *MultiAlerter) Alert(ctx context.Context, sev Severity, msg string, fields ...any) error {
    var errs []error
    for _, ch := range m.channels {
        if err := ch.Alert(ctx, sev, msg, fields...); err != nil {
            errs = append(errs, err)
        }
    }
    return errors.Join(errs...)
}

// ✅ Telegram alerter
type TelegramAlerter struct {
    botToken string
    chatID   string
    client   *http.Client
}

func (t *TelegramAlerter) Alert(ctx context.Context, sev Severity, msg string, fields ...any) error {
    text := fmt.Sprintf("[%s] %s", sev, msg)
    // ... send to Telegram API
}

// ✅ Usage trong Risk Engine
func (e *RiskEngine) checkDrawdown() {
    dd := e.calculateDrawdown()
    if dd.GreaterThanOrEqual(e.cfg.MaxDrawdownPct) {
        e.enterSafeMode()
        e.alerter.Alert(context.Background(), AlertCritical,
            "Kill switch activated",
            "drawdown", dd,
            "equity", e.equity,
            "high_water", e.highWater,
        )
    }
}
```

---

## 7. Metrics Pattern

```go
// ✅ Prometheus metrics
import "github.com/prometheus/client_golang/prometheus"

var (
    ordersTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "trading_orders_total",
            Help: "Total number of orders",
        },
        []string{"side", "status"},
    )

    equityCurrent = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "trading_equity_current",
            Help: "Current account equity",
        },
    )

    drawdownCurrent = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "trading_drawdown_current",
            Help: "Current drawdown percentage",
        },
    )

    orderLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "trading_order_latency_seconds",
            Help:    "Order execution latency",
            Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
        },
    )
)

func init() {
    prometheus.MustRegister(ordersTotal, equityCurrent, drawdownCurrent, orderLatency)
}

// ✅ Usage
func (e *Executor) PlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error) {
    start := time.Now()
    defer func() {
        orderLatency.Observe(time.Since(start).Seconds())
    }()

    result, err := e.broker.Submit(ctx, order)
    if err != nil {
        ordersTotal.WithLabelValues(order.Side.String(), "failed").Inc()
        return nil, err
    }

    ordersTotal.WithLabelValues(order.Side.String(), result.Status.String()).Inc()
    return result, nil
}
```

---

## 8. Rate Limiting Pattern

```go
// ✅ Dùng golang.org/x/time/rate
import "golang.org/x/time/rate"

type RateLimitedClient struct {
    client  BrokerClient
    limiter *rate.Limiter
}

func NewRateLimitedClient(client BrokerClient, rps int) *RateLimitedClient {
    return &RateLimitedClient{
        client:  client,
        limiter: rate.NewLimiter(rate.Limit(rps), rps),  // Token bucket
    }
}

func (r *RateLimitedClient) PlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error) {
    if err := r.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit wait: %w", err)
    }
    return r.client.PlaceOrder(ctx, order)
}

// ✅ Non-blocking check
func (r *RateLimitedClient) TryPlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error) {
    if !r.limiter.Allow() {
        return nil, ErrRateLimitExceeded
    }
    return r.client.PlaceOrder(ctx, order)
}
```

---

## 9. Structured Logging (slog)

```go
// ✅ Dùng slog (Go 1.21+) với structured fields
slog.Info("order placed",
    "order_id", order.ID,
    "client_order_id", order.ClientOrderID,
    "symbol", order.Symbol,
    "side", order.Side,
    "contracts", order.Contracts,
    "price", order.Price.String(),
)

slog.Error("order rejected",
    "order_id", order.ID,
    "reason", err.Error(),
)

slog.Warn("approaching drawdown limit",
    "current_dd", currentDD.String(),
    "max_dd", maxDD.String(),
    "equity", equity.String(),
)

// ✅ Logger với context
type contextKey string
const loggerKey contextKey = "logger"

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
    return context.WithValue(ctx, loggerKey, logger)
}

func LoggerFrom(ctx context.Context) *slog.Logger {
    if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
        return logger
    }
    return slog.Default()
}

// ❌ KHÔNG dùng string formatting
log.Printf("Order %s placed for %s: %d contracts", order.ID, order.Symbol, order.Contracts)
```

**Events bắt buộc phải log:**
- Order created/placed/filled/rejected/cancelled
- Equity update (mỗi fill)
- Kill switch activated/deactivated
- Safe mode entered/exited
- Connection lost/restored
- Graceful shutdown initiated/completed
- State persistence success/failure

---

## 10. Testing Patterns

### 10.1. Table-Driven Tests

```go
func TestPositionSizer_Calculate(t *testing.T) {
    tests := []struct {
        name          string
        equity        string  // Use string for decimal
        riskPct       string
        stopTicks     int
        tickValue     string
        wantContracts int
    }{
        {
            name:          "insufficient equity",
            equity:        "1000",
            riskPct:       "0.01",
            stopTicks:     10,
            tickValue:     "1.25",
            wantContracts: 0,
        },
        {
            name:          "normal case",
            equity:        "10000",
            riskPct:       "0.01",
            stopTicks:     10,
            tickValue:     "1.25",
            wantContracts: 8,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            sizer := NewPositionSizer(decimal.RequireFromString(tt.tickValue))
            got := sizer.Calculate(
                decimal.RequireFromString(tt.equity),
                decimal.RequireFromString(tt.riskPct),
                tt.stopTicks,
            )
            if got != tt.wantContracts {
                t.Errorf("Calculate() = %d, want %d", got, tt.wantContracts)
            }
        })
    }
}
```

### 10.2. Mock Interfaces

```go
// ✅ Mock cho testing
type MockAlerter struct {
    alerts []Alert
    mu     sync.Mutex
}

func (m *MockAlerter) Alert(ctx context.Context, sev Severity, msg string, fields ...any) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.alerts = append(m.alerts, Alert{Severity: sev, Message: msg})
    return nil
}

func (m *MockAlerter) AssertAlertSent(t *testing.T, sev Severity, msgContains string) {
    t.Helper()
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, a := range m.alerts {
        if a.Severity == sev && strings.Contains(a.Message, msgContains) {
            return
        }
    }
    t.Errorf("expected alert with severity %v containing %q", sev, msgContains)
}
```

### 10.3. Test Helpers

```go
func MustDecimal(t *testing.T, s string) decimal.Decimal {
    t.Helper()
    d, err := decimal.NewFromString(s)
    if err != nil {
        t.Fatalf("invalid decimal %q: %v", s, err)
    }
    return d
}

func AssertDecimalEqual(t *testing.T, expected, actual decimal.Decimal) {
    t.Helper()
    if !expected.Equal(actual) {
        t.Errorf("expected %s, got %s", expected, actual)
    }
}
```

---

## 11. Graceful Shutdown Pattern

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    // Initialize
    engine := risk.NewRiskEngine(cfg)
    executor := execution.NewExecutor(cfg)
    repo := persistence.NewSQLiteRepo(cfg.Persistence.Path)

    // Recover state
    if err := engine.RecoverState(ctx); err != nil {
        slog.Error("recovery failed", "err", err)
        os.Exit(1)
    }

    // Start
    go runTradingLoop(ctx, engine, executor)

    // Wait for signal
    <-ctx.Done()
    slog.Info("shutdown signal received")

    // Graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(
        context.Background(),
        time.Duration(cfg.Shutdown.TimeoutSec)*time.Second,
    )
    defer cancel()

    // Save state
    if err := repo.SaveEquitySnapshot(shutdownCtx, engine.GetSnapshot()); err != nil {
        slog.Error("failed to save state", "err", err)
    }

    // Shutdown components
    if err := engine.Shutdown(shutdownCtx); err != nil {
        slog.Error("engine shutdown error", "err", err)
    }
    if err := executor.Shutdown(shutdownCtx); err != nil {
        slog.Error("executor shutdown error", "err", err)
    }

    slog.Info("shutdown complete")
}
```

---

## 12. Module Boundaries

| Module | Responsibilities | KHÔNG được làm |
|--------|-----------------|----------------|
| `observer` | Data feed, indicators | Quyết định order |
| `strategy` | Generate Signal | Position sizing, exposure check |
| `risk` | Sizing, drawdown, exposure | Gửi order trực tiếp |
| `execution` | Gửi/cancel orders | Override risk decisions |
| `persistence` | State storage/recovery | Business logic |
| `alerting` | Send notifications | Decision making |
| `metrics` | Expose Prometheus metrics | Business logic |

**Luồng dữ liệu bắt buộc:**
```
MarketEvent → Strategy → Signal → RiskEngine → OrderIntent → Executor
                                      ↓
                                 Persistence
                                      ↓
                                  Alerting (on events)
```

---

## 13. Những điều KHÔNG làm

- ❌ Machine Learning/Deep Learning trong giai đoạn đầu
- ❌ Chiến lược phức tạp – chỉ cần đơn giản để test framework
- ❌ Tối ưu tham số để backtest "đẹp như mơ"
- ❌ Bỏ qua commission & slippage trong backtest
- ❌ Global mutable state
- ❌ Swallow errors
- ❌ Ignore context cancellation
- ❌ God interfaces
- ❌ float64 cho tiền/giá
- ❌ Duplicate orders (không có idempotency)
- ❌ Bỏ qua persistence

---

## 14. Cách Claude nên phản hồi khi được yêu cầu code

1. **Nhắc lại bối cảnh ngắn gọn** (1–2 câu) để đảm bảo hiểu đúng.
2. **Phác thảo cấu trúc** (file, type, interface) trước.
3. **Viết code từng phần nhỏ:**
   - Types & interfaces trước
   - Logic cốt lõi sau
   - Unit tests cuối cùng
4. **Giải thích vắn tắt** các quyết định thiết kế quan trọng (1–3 bullet).
5. **Không spam** giải thích trading – giữ focus vào code.

---

## 15. Quick Reference

```go
// Sentinel errors
var (
    ErrKillSwitchActive      = errors.New("kill switch active")
    ErrExposureLimitExceeded = errors.New("exposure limit exceeded")
    ErrInsufficientEquity    = errors.New("insufficient equity")
    ErrInvalidData           = errors.New("invalid market data")
    ErrOrderTimeout          = errors.New("order timeout")
    ErrDuplicateOrder        = errors.New("duplicate order")
    ErrRateLimitExceeded     = errors.New("rate limit exceeded")
    ErrPositionMismatch      = errors.New("position mismatch with broker")
)

// Core interfaces
type Strategy interface {
    OnMarketEvent(ctx context.Context, event MarketEvent) []Signal
}

type RiskEngine interface {
    ValidateAndSize(ctx context.Context, signal Signal) (*OrderIntent, error)
    UpdateEquity(snapshot EquitySnapshot)
    IsInSafeMode() bool
    Shutdown(ctx context.Context) error
}

type Executor interface {
    PlaceOrder(ctx context.Context, order OrderIntent) (*OrderResult, error)
    CancelOrder(ctx context.Context, orderID string) error
    Shutdown(ctx context.Context) error
}

type MarketDataFeed interface {
    Subscribe(ctx context.Context, symbol string) (<-chan MarketEvent, error)
    Close() error
}

type StateRepository interface {
    SaveEquitySnapshot(ctx context.Context, snapshot EquitySnapshot) error
    GetLatestEquity(ctx context.Context) (*EquitySnapshot, error)
    SavePosition(ctx context.Context, position Position) error
    GetOpenPositions(ctx context.Context) ([]Position, error)
}

type Alerter interface {
    Alert(ctx context.Context, severity Severity, message string, fields ...any) error
}
```
