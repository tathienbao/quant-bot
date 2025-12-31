# Testing Guide - Quant Trading Bot

## Triết lý

> **"Test này fail → mất tiền?"** Có → Critical. Không → Basic.

---

## Test Cases

### 1. Kill Switch (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| KS-01 | Drawdown = 20% (exact threshold) | ON | Boundary |
| KS-02 | Drawdown = 20.0001% | ON | Just over |
| KS-03 | Recovery sau kill switch (DD 20%→15%) | Vẫn ON | No auto-reset |
| KS-04 | 2 signals concurrent khi gần threshold | Max 1 pass | Race condition |
| KS-05 | Restart với kill_switch=ON saved | Vẫn ON | Persistence |
| KS-06 | Gap 50% trong 1 bar | ON ngay | Extreme gap |
| KS-07 | **HWM loaded sai (corrupt)** | Detect & alert | State corruption |
| KS-08 | **Equity update delayed 5s từ broker** | Sizing dùng stale equity | Broker lag |

### 2. Position Sizing (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| PS-01 | Standard: E=10k, risk=1%, stop=10t | 8 contracts | Formula check |
| PS-02 | Result = 8.9 contracts | 8 (floor) | Never round up |
| PS-03 | Equity = $50 (tiny) | 0, reject | Too small |
| PS-04 | Exposure limit < risk-based | Use exposure limit | Cap applies |
| PS-05 | **Đã có 2 LONG, signal thêm LONG** | Size cho total exposure | Pyramiding |
| PS-06 | **Partial fill 3/10, size cho signal mới?** | Account for partial | Mid-fill sizing |
| PS-07 | **Tick value thay đổi (contract roll)** | Use new tick value | Spec change |

### 3. Stop/Take Profit (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| SL-01 | LONG: stop < entry < tp | ✓ | Direction |
| SL-02 | SHORT: stop > entry > tp | ✓ | Direction |
| SL-03 | Bug tạo stop sai hướng | REJECT order | Defensive |
| SL-04 | Price không align tick (5000.13) | Round to 5000.25 | Exchange rule |
| SL-05 | **ATR = 0 (no data)** | Reject, don't divide by 0 | Degenerate |

### 4. Gap & Fill (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| GAP-01 | Gap qua stop (LONG@5000, stop=4990, open=4980) | Fill@4980 | Gap through |
| GAP-02 | Gap qua BOTH stop và TP | Fill@stop | Stop priority |
| GAP-03 | Limit down gap 10% | Fill@open, P&L correct | Extreme |
| GAP-04 | **Weekend gap** (Fri 5000, Sun open 4900) | Handle overnight | Weekend |
| GAP-05 | **Bar intraday: hits stop, then TP, then stop** | Fill@first hit | Order within bar |

### 5. P&L Calculation (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| PL-01 | Long profit/loss, Short profit/loss (4 cases) | Correct signs | Direction |
| PL-02 | Multiple contracts | Multiply correctly | Scale |
| PL-03 | Commission deduction | Gross - comm = Net | Fees |
| PL-04 | Scratch trade (entry=exit) | Net = -commission | Break-even |
| PL-05 | **Bust trade (broker reverses fill)** | Reverse P&L, log | Trade cancel |
| PL-06 | **Fill price wrong (broker error)** | Alert, log, manual review | Bad fill |
| PL-07 | sum(all P&L) = equity change | < $0.01 diff | Reconciliation |

### 6. Decimal Precision (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| DEC-01 | 0.1 + 0.2 | = 0.3 exactly | Float issue |
| DEC-02 | 1000 trades × $0.01 | = $10.00 | Accumulated |
| DEC-03 | Large P&L ($250,000) | No overflow | Scale |

### 7. Market Data (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| MD-01 | Invalid OHLC (high < low) | Reject bar | Bad data |
| MD-02 | Stale data (>10s old) | Don't trade | Lag |
| MD-03 | **Bad tick spike (99999 for 1 tick)** | Filter out, alert | Data error |
| MD-04 | **Data gap (miss 5 bars)** | Detect, log, continue | Gap |
| MD-05 | **Session break (16:00-17:00 CT)** | No trading, wait | CME maintenance |
| MD-06 | Duplicate timestamp | Handle or reject | Dup |

### 8. Order Lifecycle (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| ORD-01 | Submit → Fill | Happy path | Basic |
| ORD-02 | Submit when disconnected | Error immediate | No hang |
| ORD-03 | No response 30s | UNKNOWN + alert | Timeout |
| ORD-04 | Broker reject | Log reason | Reject |
| ORD-05 | **Order stuck (SUBMITTED forever)** | Timeout → cancel → alert | Stuck |
| ORD-06 | **Duplicate fill report** | Detect, ignore second | Dup fill |
| ORD-07 | **Late fill (10 min delay)** | Apply correctly, reconcile | Late |
| ORD-08 | Cancel already filled | No-op or error | Too late |

### 9. Partial Fills

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| PART-01 | Fill 3, then 7 of 10 | Position=10, avg correct | Multi-fill |
| PART-02 | Fill 5@5000, 5@5001 | Avg=5000.50 | Weighted |
| PART-03 | Partial then cancel | Keep filled portion | Partial cancel |

### 10. Broker Connection (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| BC-01 | Disconnect during order | Track state, reconcile | Network |
| BC-02 | **TWS restart while position open** | Reconnect, sync position | Broker restart |
| BC-03 | **Rate limit hit** | Backoff, queue | API limit |
| BC-04 | **Forced liquidation (margin call)** | Detect via position sync | Broker action |

### 11. Position Reconciliation (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| REC-01 | Bot=2, Broker=2 | Match | Happy |
| REC-02 | Bot=2, Broker=3 | ALERT | Qty diff |
| REC-03 | Bot=LONG, Broker=SHORT | CRITICAL ALERT, halt | Direction diff |
| REC-04 | Bot has, Broker doesn't | ALERT | Ghost |
| REC-05 | Broker has, Bot doesn't | ALERT | Unknown |
| REC-06 | **Broker shows 3, then 2 (temporary glitch)** | Wait, re-check | Transient |

### 12. Contract Rollover

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| ROLL-01 | **Expiry in 7 days** | Alert, don't open new | Near expiry |
| ROLL-02 | **Expiry in 1 day with position** | Critical alert | Must act |
| ROLL-03 | **Roll to new contract** | Close old, open new, correct specs | Rollover |

### 13. Multi-Instrument

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| MULTI-01 | **Long MES + Long MGC** | Check combined exposure | Correlation |
| MULTI-02 | **One fills, other rejects** | Handle partial state | Async fills |
| MULTI-03 | **Signal for MES while MGC order pending** | Queue or reject | Order conflict |

### 14. Time & Session

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| TIME-01 | Order T-5 sec before close | Reject or IOC | Near close |
| TIME-02 | Signal during market close | Queue or reject | Closed |
| TIME-03 | **Daylight Saving Time** | Timestamps correct | DST |
| TIME-04 | **Bot clock off by 30s** | Detect via exchange time | Clock drift |
| TIME-05 | **Holiday (market closed)** | Don't trade | Holiday |

### 15. Shutdown & Recovery (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| SHUT-01 | SIGTERM with positions | Log, save, exit <15s | Graceful |
| SHUT-02 | SIGTERM with pending orders | Cancel, wait, exit | Order cleanup |
| SHUT-03 | Order no response during shutdown | Force exit @30s | Timeout |
| SHUT-04 | **Crash (SIGKILL) with positions** | Recover on restart | Crash recovery |
| SHUT-05 | **Restart with stale state (3 days old)** | Detect, reconcile with broker | Stale state |

### 16. Concurrency (CRITICAL)

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| RACE-01 | 100 goroutines UpdateEquity() | No corruption | Write race |
| RACE-02 | ValidateAndSize() near kill switch | Atomic check | Read-write race |
| RACE-03 | GetSnapshot() during Update() | Consistent | Snapshot race |
| DEAD-01 | 24 hour continuous run | No hang | Stability |
| DEAD-02 | Shutdown during trading | No deadlock | Shutdown race |

### 17. Failure Injection

| ID | Scenario | Expected | Why |
|----|----------|----------|-----|
| FAIL-01 | Database unavailable | Trade on memory, log warn | DB fail |
| FAIL-02 | Metrics server crash | Bot continues | Non-critical fail |
| FAIL-03 | Telegram down | Log, don't block | Alert fail |
| FAIL-04 | **Disk full** | Detect, alert, continue | Storage |
| FAIL-05 | **Memory pressure** | Graceful degrade or shutdown | OOM |

---

## Invariants (PHẢI luôn đúng)

```
INV-01: sum(trades.net_pl) ≈ (end_equity - start_equity)
INV-02: position.contracts >= 0
INV-03: kill_switch=ON → no new orders (absolute)
INV-04: order.risk <= equity × max_risk_pct
INV-05: LONG: stop < entry < tp
        SHORT: stop > entry > tp
INV-06: bot_position == broker_position (eventually consistent)
INV-07: no duplicate order IDs
INV-08: HWM >= current equity (always)
```

---

## Production Checklist

### Phase 1: Before ANY trading
- [ ] Kill switch tests (KS-01 to KS-08)
- [ ] P&L calculation tests (PL-01 to PL-07)
- [ ] Position sizing tests (PS-01 to PS-07)
- [ ] Gap tests (GAP-01 to GAP-05)
- [ ] Invariant tests (INV-01 to INV-08)

### Phase 2: Before paper trading
- [ ] Reconciliation tests (REC-01 to REC-06)
- [ ] Shutdown tests (SHUT-01 to SHUT-05)
- [ ] Concurrency tests (RACE-01 to DEAD-02)
- [ ] Order lifecycle tests (ORD-01 to ORD-08)

### Phase 3: Before live trading
- [ ] Paper trade 2 weeks, 0 bugs
- [ ] Manual P&L reconciliation matches
- [ ] Graceful shutdown < 15s verified
- [ ] Contract rollover tested
- [ ] Weekend gap tested (Friday→Sunday)
- [ ] TWS restart tested
- [ ] Profitable with 2x slippage in backtest

---

## Summary

| Category | Critical Tests |
|----------|---------------|
| Kill Switch | KS-01 to KS-08 (8) |
| Position Sizing | PS-01 to PS-07 (7) |
| Stop/TP | SL-01 to SL-05 (5) |
| Gap & Fill | GAP-01 to GAP-05 (5) |
| P&L | PL-01 to PL-07 (7) |
| Decimal | DEC-01 to DEC-03 (3) |
| Market Data | MD-01 to MD-06 (6) |
| Order | ORD-01 to ORD-08 (8) |
| Partial | PART-01 to PART-03 (3) |
| Broker | BC-01 to BC-04 (4) |
| Reconciliation | REC-01 to REC-06 (6) |
| Rollover | ROLL-01 to ROLL-03 (3) |
| Multi-Instrument | MULTI-01 to MULTI-03 (3) |
| Time/Session | TIME-01 to TIME-05 (5) |
| Shutdown | SHUT-01 to SHUT-05 (5) |
| Concurrency | RACE-01 to DEAD-02 (5) |
| Failure | FAIL-01 to FAIL-05 (5) |
| **TOTAL** | **~88 tests** |

**Mới thêm (không có trong version cũ):**
- Bad tick spike, Bust trade, Forced liquidation
- TWS restart, Contract rollover, Weekend gap
- Order stuck forever, Duplicate fill, Late fill
- Multi-instrument exposure, Clock drift
- Stale state on restart, Disk full, Memory pressure
