# Function Map — Quant Bot Call Graph

> All major functions annotated with file:line, and interaction diagrams.
> Open in VSCode Markdown Preview or GitHub to render Mermaid diagrams.

---

## 1. Backtest Flow (`quant-bot backtest`)

```mermaid
flowchart TD
    A["main()<br/>cmd/bot/main.go:39"]
    B["cmdBacktest(args)<br/>cmd/bot/main.go:219"]
    C["config.Load(path)<br/>config/config.go"]
    D["observer.NewBacktestFeed(path, symbol)<br/>observer/backtest_feed.go"]
    E["observer.NewCalculator(cfg)<br/>observer/calculator.go:34"]
    F{Strategy switch}
    F1["strategy.NewGrid(cfg)<br/>strategy/grid.go"]
    F2["strategy.NewBreakout(cfg)<br/>strategy/breakout.go"]
    F3["strategy.NewMeanReversion(cfg)<br/>strategy/meanrev.go"]
    G["backtest.NewRunner(cfg, feed, calc, strat, ...)<br/>backtest/runner.go:80"]
    H["runner.Run(ctx)<br/>backtest/runner.go:116"]
    I["runner.RunSymbol(ctx, symbol)<br/>backtest/runner.go:121"]
    J["feed.Subscribe(ctx, symbol)<br/>observer/backtest_feed.go"]

    A -->|"os.Args[1]=='backtest'"| B
    B --> C
    B --> D
    B --> E
    B --> F
    F --> F1 & F2 & F3
    F1 & F2 & F3 --> G
    G --> H --> I
    I --> J

    subgraph LOOP["Per-bar Loop"]
        K["calculator.OnBar(event)<br/>observer/calculator.go:44<br/>Computes ATR, StdDev, SMA"]
        L["executor.UpdateMarket(event)<br/>execution/simulated.go<br/>Checks Stop-Loss / Take-Profit"]
        M{Fill triggered?}
        N["runner.updateEquity(fill)<br/>backtest/runner.go:224<br/>Adds NetPL to equity"]
        O["riskEngine.UpdateEquity(newEquity)<br/>risk/engine.go:193"]
        P["strategy.OnMarketEvent(ctx, event)<br/>strategy/grid.go | breakout.go | meanrev.go<br/>Returns []Signal"]
        Q["riskEngine.ValidateAndSize(ctx, signal, event)<br/>risk/engine.go:71"]
        R["executor.PlaceOrder(ctx, orderIntent)<br/>execution/simulated.go<br/>Returns OrderResult"]
        S["runner.recordEquity(timestamp, equity)<br/>backtest/runner.go:246"]
        T["progressCb(ProgressUpdate)<br/>cmd/bot/main.go:322<br/>Updates live UI"]
    end

    J --> LOOP
    K --> L --> M
    M -->|"Yes"| N --> O
    M -->|"No"| P
    O --> P
    P --> Q --> R --> S --> T

    subgraph RISK["Risk Engine — ValidateAndSize (risk/engine.go)"]
        R1["safe mode check → ErrKillSwitchActive"]
        R2["hwm.Drawdown() >= maxDD<br/>→ enterSafeModeLocked()"]
        R3["getOrCreateSizer(symbol)<br/>risk/engine.go:318"]
        R4["sizer.CalculateWithDetails(...)<br/>risk/sizer.go:87<br/>Returns SizeResult{Contracts, StopLoss}"]
        R5["checkExposureLimits(symbol, contracts, price)<br/>risk/engine.go:334"]
        R6["Returns OrderIntent{}"]
        R1 --> R2 --> R3 --> R4 --> R5 --> R6
    end

    Q --> RISK

    U["runner.calculateResults()<br/>backtest/runner.go:260<br/>Returns Result{WinRate, MaxDD, ...}"]
    V["printBacktestResults(result)<br/>cmd/bot/main.go:395"]
    W["backtest.NewMetrics(result)<br/>backtest/metrics.go:18"]
    X["printMetrics(metrics)<br/>cmd/bot/main.go:409<br/>Sharpe, Sortino, Calmar, Expectancy"]

    T --> U --> V
    U --> W --> X
```

---

## 2. Live / Paper Trading Flow (`quant-bot run`)

```mermaid
flowchart TD
    A["main()<br/>cmd/bot/main.go:39"]
    B["cmdRun(args)<br/>cmd/bot/main.go:419"]
    C["config.Load(path)<br/>config/config.go"]
    D["createAlerter(cfg, logger)<br/>cmd/bot/main.go:750<br/>Builds Console + Telegram alerter"]
    E["risk.NewEngine(cfg, equity, logger)<br/>risk/engine.go:55"]
    F["persistence.NewSQLiteRepository(path)<br/>persistence/sqlite.go"]
    F2["repo.GetState(ctx)<br/>persistence/sqlite.go<br/>Recovers saved state on restart"]
    G["paper.NewBroker(cfg, logger)<br/>broker/paper/paper.go"]
    H["paperBroker.Connect(ctx)<br/>broker/paper/paper.go"]
    I["metrics.NewServer(cfg, logger)<br/>metrics/server.go"]
    J["engine.NewEngine(cfg, broker, risk, strat, calc, alerter)<br/>engine/engine.go:59"]
    K["tradingEngine.Start(ctx)<br/>engine/engine.go:86"]

    A -->|"os.Args[1]=='run'"| B
    B --> C --> D --> E --> F --> F2
    F2 --> G --> H --> I --> J --> K

    subgraph START["engine.Start — engine/engine.go:86"]
        S1["broker.SubscribeMarketData(ctx, symbol)<br/>broker/paper/paper.go<br/>Returns marketDataCh"]
        S2["goroutine: engine.tradingLoop(ctx, marketDataCh)<br/>engine/engine.go:128"]
        S3["goroutine: engine.equityUpdateLoop(ctx)<br/>engine/engine.go:268"]
    end

    K --> START

    subgraph TRADING_LOOP["tradingLoop — engine/engine.go:128"]
        TL1["engine.processMarketEvent(ctx, event)<br/>engine/engine.go:156"]
        subgraph PME["processMarketEvent"]
            PM1["calculator.OnBar(event)<br/>observer/calculator.go:44"]
            PM2["strategy.OnMarketEvent(ctx, calcEvent)<br/>strategy/grid.go | breakout.go | meanrev.go"]
            PM3["engine.processSignal(ctx, signal, event)<br/>engine/engine.go:202"]
            subgraph PS["processSignal"]
                PS1["riskEngine.IsInSafeMode()<br/>risk/engine.go:234"]
                PS2["riskEngine.ValidateAndSize(ctx, signal, event)<br/>risk/engine.go:71<br/>Returns OrderIntent"]
                PS3["broker.PlaceOrder(ctx, orderIntent)<br/>broker/paper/paper.go"]
                PS4["alerter.Alert(SeverityInfo, 'Order placed')<br/>alerting/multi.go | telegram.go"]
                PS5["alerter.Alert(SeverityWarning, 'Order rejected')<br/>alerting/multi.go"]
            end
            PM1 --> PM2 --> PM3
            PS1 -->|"safe mode off"| PS2 --> PS3
            PS3 -->|"OK"| PS4
            PS3 -->|"Error"| PS5
        end
        TL1 --> PME
    end

    S2 --> TRADING_LOOP

    subgraph EQUITY_LOOP["equityUpdateLoop — engine/engine.go:268<br/>Ticks every 1 minute"]
        EL1["engine.updateEquity(ctx)<br/>engine/engine.go:287"]
        subgraph UE["updateEquity"]
            UE1["broker.GetAccountSummary(ctx)<br/>broker/paper/paper.go<br/>Returns AccountSummary"]
            UE2["riskEngine.UpdateEquity(netLiq)<br/>risk/engine.go:193<br/>Updates HWM, checks drawdown"]
            UE3["riskEngine.GetSnapshot()<br/>risk/engine.go:259<br/>Returns EquitySnapshot"]
            UE4["recorder.RecordEquity(equity, hwm, dd)<br/>metrics/recorder.go"]
            UE5{IsInSafeMode?}
            UE6["engine.handleKillSwitch(ctx)<br/>engine/engine.go:309"]
            UE7["alerter.Alert(SeverityCritical, 'KILL SWITCH')<br/>alerting/telegram.go"]
            UE8["engine.cancelAllOrders(ctx)<br/>engine/engine.go:329<br/>GetOpenOrders() + CancelOrder()"]
        end
        EL1 --> UE
        UE1 --> UE2 --> UE3 --> UE4 --> UE5
        UE5 -->|"Yes"| UE6 --> UE7 --> UE8
        UE5 -->|"No"| UE2
    end

    S3 --> EQUITY_LOOP

    subgraph DATA_STREAM["streamDataToPaperBroker<br/>cmd/bot/main.go:784<br/>(goroutine)"]
        DS1["observer.NewBacktestFeed(path, symbol)<br/>observer/backtest_feed.go"]
        DS2["feed.Subscribe(ctx, symbol)<br/>Returns eventCh"]
        DS3["paperBroker.SimulateMarketData(event)<br/>broker/paper/paper.go<br/>Pushes events into marketDataCh"]
    end

    H --> DATA_STREAM
    DS1 --> DS2 --> DS3

    subgraph SHUTDOWN["Graceful Shutdown"]
        SH1["tradingEngine.Stop(ctx)<br/>engine/engine.go:347<br/>close(done), wg.Wait()"]
        SH2["shutdownWithPersistence(ctx, ...)<br/>cmd/bot/main.go:685<br/>repo.SaveState()"]
        SH3["metricsServer.Shutdown(ctx)<br/>metrics/server.go"]
        SH4["alerter.Alert(SeverityInfo, 'Bot stopped')"]
    end

    B -->|"SIGINT / SIGTERM"| SHUTDOWN
```

---

## 3. Risk Engine — ValidateAndSize Internal Flow

```mermaid
flowchart TD
    IN["ValidateAndSize(ctx, signal, event)<br/>risk/engine.go:71"]

    A{ctx.Done?}
    B{safeMode?}
    C["hwm.Drawdown()<br/>risk/highwater.go"]
    D{DD >= maxDD?}
    E["enterSafeModeLocked(reason)<br/>risk/engine.go:299"]
    F["getOrCreateSizer(symbol)<br/>risk/engine.go:318"]
    G["NewPositionSizerForSymbol(symbol)<br/>risk/sizer.go:21<br/>Looks up InstrumentSpec.TickValue"]
    H["types.GetInstrumentSpec(symbol)<br/>types/types.go<br/>Returns TickSize, TickValue, MarginIntra"]
    I{stopTicks > 0?}
    J["ATR x StopLossATRMultiple / TickSize<br/>Derives stopTicks from volatility"]
    K["sizer.CalculateWithDetails(<br/>  equity, riskPct, stopTicks, price, side)<br/>risk/sizer.go:87"]
    L["capitalAtRisk = equity x riskPct<br/>contracts = floor(capitalAtRisk / tickRisk)<br/>risk/sizer.go:65"]
    M{result.Valid?}
    N["checkExposureLimits(symbol, contracts, price)<br/>risk/engine.go:334<br/>Margin check: per-symbol + total"]
    O["generateClientOrderID()<br/>risk/engine.go:389<br/>timestamp + uuid[:8]"]
    P["Returns OrderIntent{ID, Symbol, Contracts,<br/>EntryPrice, StopLoss, TakeProfit, RiskAmount}"]

    IN --> A
    A -->|"cancelled"| ERR1["return ctx.Err()"]
    A -->|"ok"| B
    B -->|"true"| ERR2["return ErrKillSwitchActive"]
    B -->|"false"| C --> D
    D -->|"yes"| E --> ERR2
    D -->|"no"| F
    F --> G --> H --> I
    I -->|"no"| J --> K
    I -->|"yes"| K
    K --> L --> M
    M -->|"invalid"| ERR3["return ErrInsufficientEquity"]
    M -->|"valid"| N
    N -->|"exceeds limit"| ERR4["return ErrExposureLimitExceeded"]
    N -->|"ok"| O --> P
```

---

## 4. Function Index by File

### `cmd/bot/main.go`
| Function | Line | Description |
|----------|------|-------------|
| `main()` | 39 | Entry point, dispatches to sub-commands |
| `cmdBacktest(args)` | 219 | Wires components and runs backtest |
| `cmdRun(args)` | 419 | Wires components and starts live/paper engine |
| `createAlerter(cfg, logger)` | 750 | Builds multi-channel alerter (console + Telegram) |
| `streamDataToPaperBroker(...)` | 784 | Streams CSV rows into paper broker at configurable speed |
| `shutdownWithPersistence(...)` | 685 | Ordered shutdown: save state → close connections |
| `printBacktestResults(result)` | 395 | Prints summary table to stdout |
| `printMetrics(metrics)` | 409 | Prints Sharpe, Sortino, Calmar, Expectancy |
| `selectStrategy()` | 95 | Interactive terminal menu for strategy selection |
| `countCSVLines(path)` | 376 | Counts data rows in CSV for progress display |

### `internal/engine/engine.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewEngine(...)` | 59 | Constructor — injects all dependencies |
| `Start(ctx)` | 86 | Subscribes to market data, launches goroutines |
| `tradingLoop(ctx, ch)` | 128 | Receives MarketEvent, drives the signal pipeline |
| `processMarketEvent(ctx, event)` | 156 | Calculator → Strategy → Signal dispatch |
| `processSignal(ctx, signal, event)` | 202 | Risk check → order placement → alerting |
| `equityUpdateLoop(ctx)` | 268 | Polls equity from broker every minute |
| `updateEquity(ctx)` | 287 | GetAccountSummary → UpdateEquity → drawdown check |
| `handleKillSwitch(ctx)` | 309 | Sends critical alert + cancels all open orders |
| `cancelAllOrders(ctx)` | 329 | Fetches and cancels every open order |
| `Stop(ctx)` | 347 | Signals goroutines to stop, waits for clean exit |

### `internal/risk/engine.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewEngine(cfg, equity, logger)` | 55 | Initialises HWM = initialEquity |
| `ValidateAndSize(ctx, signal, event)` | 71 | **Core gate** — returns OrderIntent or error |
| `UpdateEquity(equity)` | 193 | Advances HWM, triggers safe mode if drawdown exceeded |
| `UpdatePosition(position)` | 213 | Maintains in-memory position map |
| `IsInSafeMode()` | 234 | Thread-safe kill-switch status read |
| `EnterSafeMode(reason)` | 241 | Manual kill-switch trigger |
| `GetSnapshot()` | 259 | Returns current equity, HWM, drawdown ratio |
| `enterSafeModeLocked(reason)` | 299 | Internal — requires lock held by caller |
| `getOrCreateSizer(symbol)` | 318 | Lazy-initialises PositionSizer per symbol |
| `checkExposureLimits(...)` | 334 | Margin-based exposure check: per-symbol and total |
| `generateClientOrderID()` | 389 | Produces `YYYYMMDD-HHMMSS-uuid[:8]` for idempotency |

### `internal/risk/sizer.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewPositionSizer(tickValue)` | 14 | Constructor |
| `NewPositionSizerForSymbol(symbol)` | 21 | Looks up InstrumentSpec for tick value |
| `Calculate(equity, riskPct, stopTicks)` | 47 | `floor(equity × risk / stopTicks × tickValue)` |
| `CalculateWithDetails(...)` | 87 | Full result: Contracts, StopLoss price, RiskAmount |
| `MaxContracts(equity, maxPct, price, pointVal)` | 146 | Exposure ceiling in contract units |
| `AdjustForMaxSize(calculated, max)` | 173 | `min(calculated, max)` |

### `internal/risk/highwater.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewHighWaterMarkTracker(initial)` | — | Sets initial peak = starting equity |
| `Update(equity)` | — | Advances peak if equity > current peak |
| `Drawdown()` | — | Returns `(peak - current) / peak` |
| `Snapshot()` | — | Returns (current, peak, drawdown) atomically |
| `Current()` | — | Current equity value |
| `Peak()` | — | All-time high equity |

### `internal/backtest/runner.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewRunner(cfg, feed, calc, strat, ...)` | 80 | Creates runner, wires riskEngine + SimulatedExecutor |
| `Run(ctx)` | 116 | Delegates to RunSymbol("") |
| `RunSymbol(ctx, symbol)` | 121 | **Main backtest loop** — processes every bar |
| `updateEquity(equity, fill, ts)` | 224 | Adds NetPL, notifies risk engine |
| `recordEquity(ts, equity)` | 246 | Appends EquityPoint to the equity curve |
| `calculateResults()` | 260 | Aggregates WinRate, MaxDD, TotalReturn into Result |
| `Reset()` | 328 | Clears state for multi-run scenarios |

### `internal/backtest/metrics.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewMetrics(result, riskFreeRate)` | 18 | Constructor |
| `SharpeRatio()` | 28 | `(mean_return - rf) / stddev × √252` |
| `SortinoRatio()` | 54 | Uses downside deviation only |
| `MaxDrawdown()` | 77 | Scans equity curve for peak-to-trough max |
| `CalmarRatio()` | 101 | `annualReturn / maxDrawdown` |
| `AnnualizedReturn()` | 112 | `(1 + totalReturn)^(365/days) - 1` |
| `WinRate()` | 145 | `wins / totalTrades` |
| `ProfitFactor()` | 161 | `grossProfit / grossLoss` |
| `Expectancy()` | 220 | `winRate × avgWin + lossRate × avgLoss` |

### `internal/observer/calculator.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewCalculator(cfg)` | 34 | Creates ATR, StdDev, SMA indicator instances |
| `OnBar(event)` | ~44 | Feeds price into indicators, returns enriched MarketEvent |
| `CurrentATR()` | — | Returns the latest ATR value |

### `internal/execution/simulated.go`
| Function | Line | Description |
|----------|------|-------------|
| `NewSimulatedExecutor(cfg)` | 47 | Initialises positions and openOrders maps |
| `PlaceOrder(ctx, orderIntent)` | — | Stores order, enforces clientOrderID idempotency |
| `UpdateMarket(event)` | — | Checks Stop-Loss / Take-Profit for each open order |
| `GetTrades()` | — | Returns slice of closed Trade records |
| `Reset()` | — | Clears all state for a fresh run |

---

## 5. Single-Bar Data Flow

```
CSV row
  │
  ▼
BacktestFeed.Subscribe()           observer/backtest_feed.go
  │  MarketEvent{OHLCV}
  ▼
Calculator.OnBar(event)            observer/calculator.go
  │  MarketEvent{OHLCV + ATR + SMA + StdDev}
  ▼
SimulatedExecutor.UpdateMarket()   execution/simulated.go
  │  []OrderResult  (Stop-Loss / Take-Profit fills)
  ▼
Runner.updateEquity(fill)          backtest/runner.go:224
  │  newEquity = equity + NetPL
  │  RiskEngine.UpdateEquity()     risk/engine.go:193
  │    └── HWM.Update() → check drawdown → enterSafeMode?
  ▼
Strategy.OnMarketEvent(ctx, event) strategy/grid.go
  │  []Signal
  ▼
RiskEngine.ValidateAndSize()       risk/engine.go:71
  │  checks: safeMode, drawdown, position size, exposure
  │  Returns OrderIntent{Contracts, EntryPrice, StopLoss, TakeProfit}
  ▼
SimulatedExecutor.PlaceOrder()     execution/simulated.go
  │  OrderResult{Status: Filled | Rejected}
  ▼
Runner.recordEquity(ts, equity)    backtest/runner.go:246
  │  EquityPoint{Timestamp, Equity, Drawdown}
  ▼
ProgressCallback(ProgressUpdate)   cmd/bot/main.go:322
  └── BacktestUI.UpdateStats() + Render()
```
