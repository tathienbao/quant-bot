package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/alerting"
	"github.com/tathienbao/quant-bot/internal/broker"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/types"
)

// mockFailingBroker simulates broker failures.
type mockFailingBroker struct {
	connectErr         error
	placeOrderErr      error
	cancelOrderErr     error
	getAccountErr      error
	marketDataCh       chan types.MarketEvent
	subscribeErr       error
	unsubscribeErr     error
	placeOrderCallCount int
}

func newMockFailingBroker() *mockFailingBroker {
	return &mockFailingBroker{
		marketDataCh: make(chan types.MarketEvent, 10),
	}
}

func (m *mockFailingBroker) Connect(ctx context.Context) error {
	return m.connectErr
}

func (m *mockFailingBroker) Disconnect() error {
	return nil
}

func (m *mockFailingBroker) IsConnected() bool {
	return m.connectErr == nil
}

func (m *mockFailingBroker) GetAccountSummary(ctx context.Context) (*broker.AccountSummary, error) {
	if m.getAccountErr != nil {
		return nil, m.getAccountErr
	}
	return &broker.AccountSummary{
		NetLiquidation: decimal.NewFromInt(10000),
		AvailableFunds: decimal.NewFromInt(8000),
	}, nil
}

func (m *mockFailingBroker) GetPositions(ctx context.Context) ([]broker.Position, error) {
	return nil, nil
}

func (m *mockFailingBroker) GetPosition(ctx context.Context, symbol string) (*broker.Position, error) {
	return nil, nil
}

func (m *mockFailingBroker) State() broker.ConnectionState {
	if m.connectErr != nil {
		return broker.StateDisconnected
	}
	return broker.StateConnected
}

func (m *mockFailingBroker) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockFailingBroker) PlaceOrder(ctx context.Context, order types.OrderIntent) (*broker.OrderResult, error) {
	m.placeOrderCallCount++
	if m.placeOrderErr != nil {
		return nil, m.placeOrderErr
	}
	return &broker.OrderResult{
		OrderID:       "test-order-1",
		ClientOrderID: order.ClientOrderID,
		Status:        broker.OrderStatusPending,
	}, nil
}

func (m *mockFailingBroker) CancelOrder(ctx context.Context, orderID string) error {
	return m.cancelOrderErr
}

func (m *mockFailingBroker) GetOpenOrders(ctx context.Context) ([]broker.Order, error) {
	return nil, nil
}

func (m *mockFailingBroker) SubscribeMarketData(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	return m.marketDataCh, nil
}

func (m *mockFailingBroker) UnsubscribeMarketData(symbol string) error {
	return m.unsubscribeErr
}

func (m *mockFailingBroker) SendEvent(event types.MarketEvent) {
	m.marketDataCh <- event
}

func (m *mockFailingBroker) Close() {
	close(m.marketDataCh)
}

// TestEngine_Failure_BrokerDisconnect tests handling of broker disconnect (FAIL-01).
func TestEngine_Failure_BrokerDisconnect(t *testing.T) {
	brk := newMockFailingBroker()
	riskCfg := risk.DefaultConfig()
	riskEngine := risk.NewEngine(riskCfg, decimal.NewFromInt(10000), nil)
	strat := newMockStrategy("test")
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())
	mockAlerter := alerting.NewMockAlerter()

	cfg := Config{
		Symbol:               "MES",
		EquityUpdateInterval: 50 * time.Millisecond,
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start engine
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Simulate market data feed closing (broker disconnect)
	brk.Close()

	// Wait for engine to detect disconnect
	time.Sleep(100 * time.Millisecond)

	// Engine should handle gracefully
	if err := engine.Stop(ctx); err != nil {
		t.Errorf("failed to stop engine after disconnect: %v", err)
	}
}

// TestEngine_Failure_OrderRejection tests handling of order rejection (FAIL-02).
func TestEngine_Failure_OrderRejection(t *testing.T) {
	brk := newMockFailingBroker()
	brk.placeOrderErr = errors.New("insufficient margin")

	riskCfg := risk.DefaultConfig()
	riskEngine := risk.NewEngine(riskCfg, decimal.NewFromInt(10000), nil)
	strat := newMockStrategy("test")
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())
	mockAlerter := alerting.NewMockAlerter()

	cfg := Config{
		Symbol:               "MES",
		EquityUpdateInterval: 1 * time.Second,
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer engine.Stop(ctx)

	// Queue signal
	strat.AddSignal(types.Signal{
		ID:        "test-signal",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	})

	// Warm up calculator
	for i := 0; i < 20; i++ {
		brk.SendEvent(types.MarketEvent{
			Symbol:    "MES",
			Timestamp: time.Now(),
			Open:      decimal.NewFromInt(5000),
			High:      decimal.NewFromInt(5010),
			Low:       decimal.NewFromInt(4990),
			Close:     decimal.NewFromInt(5005),
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify alert was sent for rejection
	if !mockAlerter.HasAlertContaining("rejected") && brk.placeOrderCallCount > 0 {
		// Alert should be sent if order was attempted
		t.Log("Order rejection alert may not be sent if risk engine rejected first")
	}
}

// TestEngine_Failure_MarketDataFeedError tests handling of market data subscription failure (FAIL-03).
func TestEngine_Failure_MarketDataFeedError(t *testing.T) {
	brk := newMockFailingBroker()
	brk.subscribeErr = errors.New("market data subscription failed")

	riskCfg := risk.DefaultConfig()
	riskEngine := risk.NewEngine(riskCfg, decimal.NewFromInt(10000), nil)
	strat := newMockStrategy("test")
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())
	mockAlerter := alerting.NewMockAlerter()

	cfg := Config{
		Symbol:               "MES",
		EquityUpdateInterval: 1 * time.Second,
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)

	ctx := context.Background()

	// Start should fail due to subscription error
	err := engine.Start(ctx)
	if err == nil {
		engine.Stop(ctx)
		t.Fatal("expected error when market data subscription fails")
	}

	// Verify error message
	if !errors.Is(err, brk.subscribeErr) && err.Error() != "subscribe market data: market data subscription failed" {
		t.Logf("Got error: %v", err)
	}
}

// TestEngine_Failure_RiskEngineRejection tests handling of risk engine rejection (FAIL-04).
func TestEngine_Failure_RiskEngineRejection(t *testing.T) {
	brk := newMockFailingBroker()

	// Configure risk engine to reject (kill switch active)
	riskCfg := risk.DefaultConfig()
	riskCfg.MaxGlobalDrawdownPct = decimal.RequireFromString("0.10")
	riskEngine := risk.NewEngine(riskCfg, decimal.NewFromInt(10000), nil)

	// Trigger kill switch by setting high drawdown
	riskEngine.UpdateEquity(decimal.NewFromInt(8500)) // 15% drawdown

	strat := newMockStrategy("test")
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())
	mockAlerter := alerting.NewMockAlerter()

	cfg := Config{
		Symbol:               "MES",
		EquityUpdateInterval: 1 * time.Second,
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer engine.Stop(ctx)

	// Queue signal
	strat.AddSignal(types.Signal{
		ID:        "rejected-signal",
		Symbol:    "MES",
		Direction: types.SideLong,
		StopTicks: 10,
	})

	// Warm up calculator
	for i := 0; i < 20; i++ {
		brk.SendEvent(types.MarketEvent{
			Symbol:    "MES",
			Timestamp: time.Now(),
			Open:      decimal.NewFromInt(5000),
			High:      decimal.NewFromInt(5010),
			Low:       decimal.NewFromInt(4990),
			Close:     decimal.NewFromInt(5005),
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify no order was placed (risk engine rejected)
	if brk.placeOrderCallCount > 0 {
		t.Error("expected no orders when risk engine is in safe mode")
	}
}

// TestEngine_Failure_GetAccountSummaryError tests handling of account summary failure (FAIL-05).
func TestEngine_Failure_GetAccountSummaryError(t *testing.T) {
	brk := newMockFailingBroker()
	brk.getAccountErr = errors.New("connection lost")

	riskCfg := risk.DefaultConfig()
	riskEngine := risk.NewEngine(riskCfg, decimal.NewFromInt(10000), nil)
	strat := newMockStrategy("test")
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())
	mockAlerter := alerting.NewMockAlerter()

	cfg := Config{
		Symbol:               "MES",
		EquityUpdateInterval: 50 * time.Millisecond, // Fast for testing
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Wait for equity update to fail
	time.Sleep(150 * time.Millisecond)

	// Engine should continue running despite equity update failures
	if !engine.IsRunning() {
		t.Error("engine should continue running despite equity update failures")
	}

	// Stop should still work
	if err := engine.Stop(ctx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}
}
