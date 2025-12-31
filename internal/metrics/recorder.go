package metrics

import (
	"time"

	"github.com/shopspring/decimal"
)

// Recorder provides methods for recording metrics.
type Recorder struct{}

// NewRecorder creates a new metrics recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// RecordOrder records an order metric.
func (r *Recorder) RecordOrder(symbol, side, status string) {
	OrdersTotal.WithLabelValues(symbol, side, status).Inc()
}

// RecordTrade records a completed trade metric.
func (r *Recorder) RecordTrade(symbol, side string, profitable bool) {
	outcome := "loss"
	if profitable {
		outcome = "win"
	}
	TradesTotal.WithLabelValues(symbol, side, outcome).Inc()
}

// RecordPositionOpened records a position being opened.
func (r *Recorder) RecordPositionOpened(symbol, side string, contracts int) {
	PositionsOpen.WithLabelValues(symbol).Inc()
	PositionContracts.WithLabelValues(symbol, side).Add(float64(contracts))
}

// RecordPositionClosed records a position being closed.
func (r *Recorder) RecordPositionClosed(symbol, side string, contracts int) {
	PositionsOpen.WithLabelValues(symbol).Dec()
	PositionContracts.WithLabelValues(symbol, side).Sub(float64(contracts))
}

// RecordEquity records equity metrics.
func (r *Recorder) RecordEquity(current, highWaterMark, drawdown decimal.Decimal) {
	EquityCurrent.Set(current.InexactFloat64())
	EquityHighWaterMark.Set(highWaterMark.InexactFloat64())
	DrawdownCurrent.Set(drawdown.InexactFloat64())
}

// RecordDailyPL records daily profit/loss.
func (r *Recorder) RecordDailyPL(pl decimal.Decimal) {
	DailyPL.Set(pl.InexactFloat64())
}

// RecordTotalPL records total profit/loss.
func (r *Recorder) RecordTotalPL(pl decimal.Decimal) {
	TotalPL.Set(pl.InexactFloat64())
}

// RecordSafeMode records safe mode status.
func (r *Recorder) RecordSafeMode(active bool) {
	if active {
		SafeModeActive.Set(1)
	} else {
		SafeModeActive.Set(0)
	}
}

// RecordSignal records a signal being generated.
func (r *Recorder) RecordSignal(strategy, side string) {
	SignalsGenerated.WithLabelValues(strategy, side).Inc()
}

// RecordSignalRejected records a signal being rejected.
func (r *Recorder) RecordSignalRejected(reason string) {
	SignalsRejected.WithLabelValues(reason).Inc()
}

// RecordOrderLatency records order execution latency.
func (r *Recorder) RecordOrderLatency(duration time.Duration) {
	OrderLatency.Observe(duration.Seconds())
}

// RecordDataFeedLatency records data feed latency.
func (r *Recorder) RecordDataFeedLatency(duration time.Duration) {
	DataFeedLatency.Observe(duration.Seconds())
}

// RecordStrategyLatency records strategy computation latency.
func (r *Recorder) RecordStrategyLatency(strategy string, duration time.Duration) {
	StrategyLatency.WithLabelValues(strategy).Observe(duration.Seconds())
}

// RecordHeartbeat records a heartbeat.
func (r *Recorder) RecordHeartbeat() {
	HeartbeatTimestamp.Set(float64(time.Now().Unix()))
}

// RecordDataFeedStatus records data feed connection status.
func (r *Recorder) RecordDataFeedStatus(connected bool) {
	if connected {
		DataFeedConnected.Set(1)
	} else {
		DataFeedConnected.Set(0)
	}
}

// RecordBrokerStatus records broker connection status.
func (r *Recorder) RecordBrokerStatus(connected bool) {
	if connected {
		BrokerConnected.Set(1)
	} else {
		BrokerConnected.Set(0)
	}
}

// RecordError records an error.
func (r *Recorder) RecordError(errorType string) {
	ErrorsTotal.WithLabelValues(errorType).Inc()
}

// Timer is a helper for measuring latency.
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer.
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Elapsed returns the elapsed duration.
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}

// ObserveOrder observes the elapsed time as order latency.
func (t *Timer) ObserveOrder() {
	OrderLatency.Observe(t.Elapsed().Seconds())
}

// ObserveStrategy observes the elapsed time as strategy latency.
func (t *Timer) ObserveStrategy(strategy string) {
	StrategyLatency.WithLabelValues(strategy).Observe(t.Elapsed().Seconds())
}
