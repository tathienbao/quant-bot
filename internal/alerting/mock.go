package alerting

import (
	"context"
	"strings"
	"sync"
)

// MockAlerter is a mock alerter for testing.
type MockAlerter struct {
	mu     sync.Mutex
	alerts []MockAlert
}

// MockAlert represents a captured alert for testing.
type MockAlert struct {
	Severity Severity
	Message  string
	Fields   []any
}

// NewMockAlerter creates a new mock alerter.
func NewMockAlerter() *MockAlerter {
	return &MockAlerter{
		alerts: make([]MockAlert, 0),
	}
}

// Name returns the name of the alerter.
func (m *MockAlerter) Name() string {
	return "mock"
}

// Alert captures the alert for later verification.
func (m *MockAlerter) Alert(_ context.Context, severity Severity, message string, fields ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, MockAlert{
		Severity: severity,
		Message:  message,
		Fields:   fields,
	})
	return nil
}

// Alerts returns all captured alerts.
func (m *MockAlerter) Alerts() []MockAlert {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockAlert, len(m.alerts))
	copy(result, m.alerts)
	return result
}

// Clear clears all captured alerts.
func (m *MockAlerter) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = m.alerts[:0]
}

// Count returns the number of captured alerts.
func (m *MockAlerter) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.alerts)
}

// HasAlertWithSeverity checks if an alert with the given severity was sent.
func (m *MockAlerter) HasAlertWithSeverity(severity Severity) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if a.Severity == severity {
			return true
		}
	}
	return false
}

// HasAlertContaining checks if an alert containing the message substring was sent.
func (m *MockAlerter) HasAlertContaining(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.alerts {
		if strings.Contains(a.Message, substr) {
			return true
		}
	}
	return false
}

// LastAlert returns the last captured alert, or nil if none.
func (m *MockAlerter) LastAlert() *MockAlert {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.alerts) == 0 {
		return nil
	}
	last := m.alerts[len(m.alerts)-1]
	return &last
}
