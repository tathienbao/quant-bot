package observer

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// mockFeed implements MarketDataFeed for testing.
type mockFeed struct {
	events []types.MarketEvent
	closed bool
}

func newMockFeed(events []types.MarketEvent) *mockFeed {
	return &mockFeed{events: events}
}

func (m *mockFeed) Subscribe(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	ch := make(chan types.MarketEvent, len(m.events))
	go func() {
		defer close(ch)
		for _, event := range m.events {
			if event.Symbol != symbol {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- event:
			}
		}
	}()
	return ch, nil
}

func (m *mockFeed) Close() error {
	m.closed = true
	return nil
}

func (m *mockFeed) Name() string {
	return "mock"
}

// mockCalculator implements IndicatorCalculator for testing.
type mockCalculator struct {
	callCount int
}

func (m *mockCalculator) OnBar(event types.MarketEvent) types.MarketEvent {
	m.callCount++
	event.ATR = decimal.NewFromInt(10) // Add mock ATR
	return event
}

func (m *mockCalculator) Reset() {
	m.callCount = 0
}

// TestNewObserver tests observer constructor.
func TestNewObserver(t *testing.T) {
	feed := newMockFeed(nil)
	calc := &mockCalculator{}

	obs := NewObserver(feed, calc)
	if obs == nil {
		t.Fatal("expected observer to be created")
	}
}

// TestObserver_Subscribe tests subscription flow.
func TestObserver_Subscribe(t *testing.T) {
	events := []types.MarketEvent{
		{
			Timestamp: time.Now(),
			Symbol:    "MES",
			Open:      decimal.NewFromInt(5000),
			High:      decimal.NewFromInt(5010),
			Low:       decimal.NewFromInt(4990),
			Close:     decimal.NewFromInt(5005),
			Volume:    1000,
		},
		{
			Timestamp: time.Now().Add(5 * time.Minute),
			Symbol:    "MES",
			Open:      decimal.NewFromInt(5005),
			High:      decimal.NewFromInt(5015),
			Low:       decimal.NewFromInt(5000),
			Close:     decimal.NewFromInt(5010),
			Volume:    1200,
		},
	}

	feed := newMockFeed(events)
	calc := &mockCalculator{}
	obs := NewObserver(feed, calc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := obs.Subscribe(ctx, "MES")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Receive events
	received := 0
	for range ch {
		received++
	}

	if received != 2 {
		t.Errorf("expected 2 events, got %d", received)
	}

	// Calculator should have been called for each event
	if calc.callCount != 2 {
		t.Errorf("expected calculator to be called 2 times, got %d", calc.callCount)
	}
}

// TestObserver_Subscribe_ContextCancelled tests cancellation handling.
func TestObserver_Subscribe_ContextCancelled(t *testing.T) {
	// Create many events
	events := make([]types.MarketEvent, 100)
	for i := range events {
		events[i] = types.MarketEvent{
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Symbol:    "MES",
			Close:     decimal.NewFromInt(5000),
		}
	}

	feed := newMockFeed(events)
	calc := &mockCalculator{}
	obs := NewObserver(feed, calc)

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := obs.Subscribe(ctx, "MES")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Receive a few events then cancel
	received := 0
	for event := range ch {
		received++
		if received >= 5 {
			cancel()
			break
		}
		_ = event
	}

	// Drain remaining events
	for range ch {
	}

	// Should have received at least 5 but not all 100
	if received < 5 {
		t.Errorf("expected at least 5 events, got %d", received)
	}
}

// TestObserver_Close tests resource cleanup.
func TestObserver_Close(t *testing.T) {
	feed := newMockFeed(nil)
	calc := &mockCalculator{}
	obs := NewObserver(feed, calc)

	err := obs.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !feed.closed {
		t.Error("expected feed to be closed")
	}
}

// TestObserver_Reset tests state reset.
func TestObserver_Reset(t *testing.T) {
	feed := newMockFeed(nil)
	calc := &mockCalculator{}
	calc.callCount = 10 // Simulate some calls
	obs := NewObserver(feed, calc)

	obs.Reset()

	if calc.callCount != 0 {
		t.Error("expected calculator to be reset")
	}
}

// TestObserver_Subscribe_WithEnrichment tests indicator enrichment.
func TestObserver_Subscribe_WithEnrichment(t *testing.T) {
	events := []types.MarketEvent{
		{
			Timestamp: time.Now(),
			Symbol:    "MES",
			Close:     decimal.NewFromInt(5000),
			ATR:       decimal.Zero, // No ATR initially
		},
	}

	feed := newMockFeed(events)
	calc := &mockCalculator{} // Will add ATR=10
	obs := NewObserver(feed, calc)

	ctx := context.Background()
	ch, _ := obs.Subscribe(ctx, "MES")

	event := <-ch

	// Calculator should have enriched the event
	if !event.ATR.Equal(decimal.NewFromInt(10)) {
		t.Errorf("expected ATR=10, got %s", event.ATR.String())
	}
}
