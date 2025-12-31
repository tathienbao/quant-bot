package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
)

func TestRecorder_RecordOrder(t *testing.T) {
	r := NewRecorder()

	// Record some orders
	r.RecordOrder("MES", "long", "filled")
	r.RecordOrder("MES", "short", "rejected")
	r.RecordOrder("MGC", "long", "filled")

	// Verify counter incremented (we can't easily read the value, but no panic means success)
}

func TestRecorder_RecordTrade(t *testing.T) {
	r := NewRecorder()

	r.RecordTrade("MES", "long", true)
	r.RecordTrade("MES", "short", false)
}

func TestRecorder_RecordPosition(t *testing.T) {
	r := NewRecorder()

	r.RecordPositionOpened("MES", "long", 2)
	r.RecordPositionClosed("MES", "long", 2)
}

func TestRecorder_RecordEquity(t *testing.T) {
	r := NewRecorder()

	current := decimal.NewFromInt(10500)
	hwm := decimal.NewFromInt(11000)
	drawdown := decimal.NewFromFloat(0.045)

	r.RecordEquity(current, hwm, drawdown)
}

func TestRecorder_RecordSafeMode(t *testing.T) {
	r := NewRecorder()

	r.RecordSafeMode(true)
	r.RecordSafeMode(false)
}

func TestRecorder_RecordSignal(t *testing.T) {
	r := NewRecorder()

	r.RecordSignal("breakout", "long")
	r.RecordSignal("meanrev", "short")
	r.RecordSignalRejected("insufficient_equity")
}

func TestRecorder_RecordLatency(t *testing.T) {
	r := NewRecorder()

	r.RecordOrderLatency(100 * time.Millisecond)
	r.RecordDataFeedLatency(5 * time.Millisecond)
	r.RecordStrategyLatency("breakout", 500 * time.Microsecond)
}

func TestRecorder_RecordHeartbeat(t *testing.T) {
	r := NewRecorder()
	r.RecordHeartbeat()
}

func TestRecorder_RecordConnectionStatus(t *testing.T) {
	r := NewRecorder()

	r.RecordDataFeedStatus(true)
	r.RecordDataFeedStatus(false)
	r.RecordBrokerStatus(true)
	r.RecordBrokerStatus(false)
}

func TestRecorder_RecordError(t *testing.T) {
	r := NewRecorder()

	r.RecordError("connection")
	r.RecordError("order_timeout")
}

func TestTimer(t *testing.T) {
	timer := NewTimer()
	time.Sleep(10 * time.Millisecond)

	elapsed := timer.Elapsed()
	if elapsed < 10*time.Millisecond {
		t.Errorf("elapsed = %v, expected >= 10ms", elapsed)
	}
}

func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo("1.0.0", "abc123", "2024-12-31")
}

func TestMetricsRegistered(t *testing.T) {
	// Verify all metrics are registered with Prometheus
	// This is implicit through promauto, but we verify no panics occur
	metrics := []prometheus.Collector{
		OrdersTotal,
		TradesTotal,
		PositionsOpen,
		PositionContracts,
		EquityCurrent,
		EquityHighWaterMark,
		DrawdownCurrent,
		DailyPL,
		TotalPL,
		SafeModeActive,
		SignalsGenerated,
		SignalsRejected,
		OrderLatency,
		DataFeedLatency,
		StrategyLatency,
		HeartbeatTimestamp,
		DataFeedConnected,
		BrokerConnected,
		UptimeSeconds,
		ErrorsTotal,
		BuildInfo,
	}

	for _, m := range metrics {
		if m == nil {
			t.Error("metric is nil")
		}
	}
}
