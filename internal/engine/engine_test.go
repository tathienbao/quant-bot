package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/alerting"
	"github.com/tathienbao/quant-bot/internal/broker/paper"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/types"
)

// mockStrategy implements strategy.Strategy for testing.
type mockStrategy struct {
	mu       sync.Mutex
	name     string
	signals  []types.Signal
	callCount int
}

func newMockStrategy(name string) *mockStrategy {
	return &mockStrategy{
		name:    name,
		signals: make([]types.Signal, 0),
	}
}

func (m *mockStrategy) OnMarketEvent(_ context.Context, _ types.MarketEvent) []types.Signal {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++

	if len(m.signals) > 0 {
		sig := m.signals[0]
		m.signals = m.signals[1:]
		return []types.Signal{sig}
	}
	return nil
}

func (m *mockStrategy) Name() string {
	return m.name
}

func (m *mockStrategy) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals = m.signals[:0]
	m.callCount = 0
}

func (m *mockStrategy) AddSignal(sig types.Signal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals = append(m.signals, sig)
}

func (m *mockStrategy) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// Test helpers
func createTestEngine(t *testing.T) (*Engine, *paper.Broker, *mockStrategy, *alerting.MockAlerter) {
	t.Helper()

	// Create paper broker
	brokerCfg := paper.DefaultConfig()
	brokerCfg.InitialEquity = decimal.NewFromInt(10000)
	brk := paper.NewBroker(brokerCfg, nil)

	// Create risk engine
	riskCfg := risk.DefaultConfig()
	initialEquity := decimal.NewFromInt(10000)
	riskEngine := risk.NewEngine(riskCfg, initialEquity, nil)

	// Create mock strategy
	strat := newMockStrategy("test_strategy")

	// Create calculator
	calc := observer.NewCalculator(observer.DefaultCalculatorConfig())

	// Create mock alerter
	mockAlerter := alerting.NewMockAlerter()

	// Create engine config
	cfg := Config{
		Symbol:               "MES",
		Timeframe:            5 * time.Minute,
		EquityUpdateInterval: 100 * time.Millisecond, // Fast for testing
	}

	engine := NewEngine(cfg, brk, riskEngine, strat, calc, mockAlerter, nil)
	return engine, brk, strat, mockAlerter
}

// TestNewEngine tests engine constructor.
func TestNewEngine(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)

	if engine == nil {
		t.Fatal("expected engine to be created")
	}

	if engine.IsRunning() {
		t.Error("expected engine to not be running initially")
	}

	if engine.cfg.Symbol != "MES" {
		t.Errorf("expected symbol MES, got %s", engine.cfg.Symbol)
	}
}

// TestEngine_Start_Success tests successful engine start (ORD-01 happy path).
func TestEngine_Start_Success(t *testing.T) {
	engine, brk, _, mockAlerter := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect broker first
	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// Start engine
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Verify running
	if !engine.IsRunning() {
		t.Error("expected engine to be running")
	}

	// Verify start alert was sent
	if !mockAlerter.HasAlertContaining("started") {
		t.Error("expected start alert to be sent")
	}

	// Clean up
	if err := engine.Stop(ctx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}
}

// TestEngine_Start_AlreadyRunning tests double start prevention.
func TestEngine_Start_AlreadyRunning(t *testing.T) {
	engine, brk, _, _ := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// First start
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}
	defer engine.Stop(ctx)

	// Second start should fail
	if err := engine.Start(ctx); err == nil {
		t.Error("expected error when starting already running engine")
	}
}

// TestEngine_Stop_Graceful tests graceful shutdown (SHUT-01).
func TestEngine_Stop_Graceful(t *testing.T) {
	engine, brk, _, mockAlerter := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Measure shutdown time
	start := time.Now()
	if err := engine.Stop(ctx); err != nil {
		t.Errorf("failed to stop engine: %v", err)
	}
	duration := time.Since(start)

	// SHUT-01: graceful shutdown < 15s
	if duration > 15*time.Second {
		t.Errorf("shutdown took too long: %v (expected < 15s)", duration)
	}

	// Verify stopped
	if engine.IsRunning() {
		t.Error("expected engine to be stopped")
	}

	// Verify stop alert was sent
	if !mockAlerter.HasAlertContaining("stopped") {
		t.Error("expected stop alert to be sent")
	}
}

// TestEngine_Stop_NotRunning tests stopping non-running engine.
func TestEngine_Stop_NotRunning(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)
	ctx := context.Background()

	// Stop when not running should be no-op
	if err := engine.Stop(ctx); err != nil {
		t.Errorf("unexpected error stopping non-running engine: %v", err)
	}
}

// TestEngine_ProcessSignal_SafeMode tests signal rejection in safe mode (KS-03).
func TestEngine_ProcessSignal_SafeMode(t *testing.T) {
	engine, brk, strat, _ := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// Manually trigger safe mode by simulating large drawdown
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(10000)) // Set HWM
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(7500))  // 25% drawdown > 20% threshold

	// Verify safe mode is active
	if !engine.riskEngine.IsInSafeMode() {
		t.Fatal("expected safe mode to be active")
	}

	// Add a signal
	strat.AddSignal(types.Signal{
		ID:           "test-signal-1",
		Symbol:       "MES",
		Direction:    types.SideLong,
		StopTicks:    10,
		StrategyName: "test_strategy",
	})

	// Create market event
	event := types.MarketEvent{
		Timestamp: time.Now(),
		Symbol:    "MES",
		Open:      decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5010),
		Low:       decimal.NewFromInt(4990),
		Close:     decimal.NewFromInt(5005),
		Volume:    1000,
		ATR:       decimal.NewFromInt(10),
	}

	// Process signal - should be rejected due to safe mode
	err := engine.processSignal(ctx, strat.signals[0], event)
	if err != types.ErrKillSwitchActive {
		t.Errorf("expected ErrKillSwitchActive, got: %v", err)
	}
}

// TestEngine_IsRunning tests running state queries.
func TestEngine_IsRunning(t *testing.T) {
	engine, brk, _, _ := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if engine.IsRunning() {
		t.Error("expected engine to not be running initially")
	}

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	if !engine.IsRunning() {
		t.Error("expected engine to be running after start")
	}

	if err := engine.Stop(ctx); err != nil {
		t.Fatalf("failed to stop engine: %v", err)
	}

	if engine.IsRunning() {
		t.Error("expected engine to not be running after stop")
	}
}

// TestEngine_GetLastEvent tests last event retrieval.
func TestEngine_GetLastEvent(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)

	// Initially empty
	event := engine.GetLastEvent()
	if !event.Timestamp.IsZero() {
		t.Error("expected empty last event initially")
	}

	// Set event via processMarketEvent
	testEvent := types.MarketEvent{
		Timestamp: time.Now(),
		Symbol:    "MES",
		Open:      decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5010),
		Low:       decimal.NewFromInt(4990),
		Close:     decimal.NewFromInt(5005),
		Volume:    1000,
	}

	ctx := context.Background()
	_ = engine.processMarketEvent(ctx, testEvent)

	retrieved := engine.GetLastEvent()
	if retrieved.Symbol != testEvent.Symbol {
		t.Errorf("expected symbol %s, got %s", testEvent.Symbol, retrieved.Symbol)
	}
}

// TestEngine_ContextCancelled tests context cancellation handling.
func TestEngine_ContextCancelled(t *testing.T) {
	engine, brk, _, _ := createTestEngine(t)
	ctx, cancel := context.WithCancel(context.Background())

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("failed to start engine: %v", err)
	}

	// Cancel context
	cancel()

	// Give time for goroutines to stop
	time.Sleep(100 * time.Millisecond)

	// Engine should handle cancellation gracefully
	// (trading loop should exit on ctx.Done())
}

// TestEngine_ProcessMarketEvent tests market event processing.
func TestEngine_ProcessMarketEvent(t *testing.T) {
	engine, _, strat, _ := createTestEngine(t)
	ctx := context.Background()

	event := types.MarketEvent{
		Timestamp: time.Now(),
		Symbol:    "MES",
		Open:      decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5010),
		Low:       decimal.NewFromInt(4990),
		Close:     decimal.NewFromInt(5005),
		Volume:    1000,
	}

	// Process event
	if err := engine.processMarketEvent(ctx, event); err != nil {
		t.Errorf("unexpected error processing market event: %v", err)
	}

	// Verify strategy was called
	if strat.CallCount() != 1 {
		t.Errorf("expected strategy to be called once, got %d", strat.CallCount())
	}

	// Verify last event updated
	lastEvent := engine.GetLastEvent()
	if lastEvent.Symbol != event.Symbol {
		t.Error("expected last event to be updated")
	}
}

// TestEngine_ProcessMarketEvent_WithSignal tests market event with signal generation.
func TestEngine_ProcessMarketEvent_WithSignal(t *testing.T) {
	engine, brk, strat, _ := createTestEngine(t)
	ctx := context.Background()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// Add a signal to be returned
	strat.AddSignal(types.Signal{
		ID:           "test-signal-1",
		Symbol:       "MES",
		Direction:    types.SideLong,
		StopTicks:    10,
		StrategyName: "test_strategy",
		Strength:     decimal.NewFromFloat(0.8),
	})

	event := types.MarketEvent{
		Timestamp: time.Now(),
		Symbol:    "MES",
		Open:      decimal.NewFromInt(5000),
		High:      decimal.NewFromInt(5010),
		Low:       decimal.NewFromInt(4990),
		Close:     decimal.NewFromInt(5005),
		Volume:    1000,
		ATR:       decimal.NewFromInt(10),
	}

	// Process event (will generate and process signal)
	if err := engine.processMarketEvent(ctx, event); err != nil {
		t.Errorf("unexpected error processing market event: %v", err)
	}

	// Signal should have been consumed
	if len(strat.signals) != 0 {
		t.Error("expected signal to be consumed")
	}
}

// TestEngine_CancelAllOrders tests order cancellation.
func TestEngine_CancelAllOrders(t *testing.T) {
	engine, brk, _, _ := createTestEngine(t)
	ctx := context.Background()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// Cancel all orders (should handle empty orders gracefully)
	engine.cancelAllOrders(ctx)

	// Verify no panic and no errors (basic sanity test)
}

// TestEngine_HandleKillSwitch tests kill switch handling.
func TestEngine_HandleKillSwitch(t *testing.T) {
	engine, brk, _, mockAlerter := createTestEngine(t)
	ctx := context.Background()

	if err := brk.Connect(ctx); err != nil {
		t.Fatalf("failed to connect broker: %v", err)
	}

	// Clear previous alerts
	mockAlerter.Clear()

	// Handle kill switch
	engine.handleKillSwitch(ctx)

	// Verify critical alert was sent
	if !mockAlerter.HasAlertWithSeverity(alerting.SeverityCritical) {
		t.Error("expected critical alert for kill switch")
	}

	if !mockAlerter.HasAlertContaining("KILL SWITCH") {
		t.Error("expected alert to contain 'KILL SWITCH'")
	}
}

// TestEngine_KillSwitch_ExactThreshold tests exact 20% threshold (KS-01).
func TestEngine_KillSwitch_ExactThreshold(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)

	// Set HWM to 10000
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(10000))

	// Exact 20% drawdown: 10000 -> 8000
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(8000))

	// Should trigger safe mode at exactly 20%
	if !engine.riskEngine.IsInSafeMode() {
		t.Error("expected safe mode at exact 20% threshold (KS-01)")
	}
}

// TestEngine_KillSwitch_JustOver tests 20.0001% threshold (KS-02).
func TestEngine_KillSwitch_JustOver(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)

	// Set HWM to 10000
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(10000))

	// Just over 20% drawdown: 10000 -> 7999.99
	engine.riskEngine.UpdateEquity(decimal.RequireFromString("7999.99"))

	// Should trigger safe mode
	if !engine.riskEngine.IsInSafeMode() {
		t.Error("expected safe mode at 20.0001% threshold (KS-02)")
	}
}

// TestEngine_KillSwitch_Recovery tests no auto-reset after recovery (KS-03).
func TestEngine_KillSwitch_Recovery(t *testing.T) {
	engine, _, _, _ := createTestEngine(t)

	// Trigger safe mode
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(10000))
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(7500)) // 25% DD

	if !engine.riskEngine.IsInSafeMode() {
		t.Fatal("expected safe mode to be triggered")
	}

	// Recover equity to 15% DD
	engine.riskEngine.UpdateEquity(decimal.NewFromInt(8500))

	// KS-03: Safe mode should still be ON (no auto-reset)
	if !engine.riskEngine.IsInSafeMode() {
		t.Error("expected safe mode to remain ON after recovery (KS-03)")
	}
}
