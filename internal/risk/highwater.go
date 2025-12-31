// Package risk implements the risk management engine.
package risk

import (
	"sync"

	"github.com/shopspring/decimal"
)

// HighWaterMarkTracker tracks the peak equity value.
// Thread-safe for concurrent access.
type HighWaterMarkTracker struct {
	mu      sync.RWMutex
	peak    decimal.Decimal
	current decimal.Decimal
}

// NewHighWaterMarkTracker creates a new tracker with initial equity.
func NewHighWaterMarkTracker(initialEquity decimal.Decimal) *HighWaterMarkTracker {
	return &HighWaterMarkTracker{
		peak:    initialEquity,
		current: initialEquity,
	}
}

// Update updates the current equity and adjusts the peak if necessary.
// Returns true if a new peak was set.
func (h *HighWaterMarkTracker) Update(equity decimal.Decimal) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.current = equity

	if equity.GreaterThan(h.peak) {
		h.peak = equity
		return true
	}

	return false
}

// Current returns the current equity value.
func (h *HighWaterMarkTracker) Current() decimal.Decimal {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// Peak returns the high water mark (peak equity).
func (h *HighWaterMarkTracker) Peak() decimal.Decimal {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.peak
}

// Drawdown calculates the current drawdown as a ratio.
// Returns (peak - current) / peak
// A value of 0.15 means 15% drawdown.
func (h *HighWaterMarkTracker) Drawdown() decimal.Decimal {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.peak.IsZero() {
		return decimal.Zero
	}

	if h.current.GreaterThanOrEqual(h.peak) {
		return decimal.Zero
	}

	// DD = (peak - current) / peak
	return h.peak.Sub(h.current).Div(h.peak)
}

// Reset resets the tracker to a new initial equity.
// Use with caution - typically only for testing or manual reset.
func (h *HighWaterMarkTracker) Reset(equity decimal.Decimal) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.peak = equity
	h.current = equity
}

// Snapshot returns the current state as a copy.
func (h *HighWaterMarkTracker) Snapshot() (current, peak, drawdown decimal.Decimal) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	current = h.current
	peak = h.peak

	if h.peak.IsZero() || h.current.GreaterThanOrEqual(h.peak) {
		drawdown = decimal.Zero
	} else {
		drawdown = h.peak.Sub(h.current).Div(h.peak)
	}

	return current, peak, drawdown
}
