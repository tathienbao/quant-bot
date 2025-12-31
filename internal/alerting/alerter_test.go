package alerting

import (
	"context"
	"testing"
)

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityInfo, "INFO"},
		{SeverityWarning, "WARNING"},
		{SeverityHigh, "HIGH"},
		{SeverityCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.severity.String(); got != tt.want {
				t.Errorf("Severity.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeverity_Emoji(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityInfo, "ℹ️"},
		{SeverityWarning, "⚠️"},
		{SeverityHigh, "🔴"},
		{SeverityCritical, "🚨"},
		{Severity(99), "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.severity.String(), func(t *testing.T) {
			if got := tt.severity.Emoji(); got != tt.want {
				t.Errorf("Severity.Emoji() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatFields(t *testing.T) {
	tests := []struct {
		name   string
		fields []any
		want   string
	}{
		{
			name:   "empty fields",
			fields: nil,
			want:   "",
		},
		{
			name:   "single field",
			fields: []any{"key", "value"},
			want:   "• key: value",
		},
		{
			name:   "multiple fields",
			fields: []any{"key1", "value1", "key2", 123},
			want:   "• key1: value1\n• key2: 123",
		},
		{
			name:   "odd number of fields",
			fields: []any{"key1", "value1", "orphan"},
			want:   "• key1: value1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatFields(tt.fields...); got != tt.want {
				t.Errorf("FormatFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEventSeverity(t *testing.T) {
	tests := []struct {
		event AlertEvent
		want  Severity
	}{
		{EventKillSwitchActivated, SeverityCritical},
		{EventSafeModeEntered, SeverityHigh},
		{EventSafeModeExited, SeverityHigh},
		{EventOrderRejected, SeverityWarning},
		{EventConnectionLost, SeverityWarning},
		{EventOrderFilled, SeverityInfo},
		{EventPositionOpened, SeverityInfo},
		{EventPositionClosed, SeverityInfo},
		{EventDailySummary, SeverityInfo},
		{EventBotStarted, SeverityInfo},
		{EventBotStopped, SeverityInfo},
		{AlertEvent("unknown"), SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			if got := EventSeverity(tt.event); got != tt.want {
				t.Errorf("EventSeverity(%s) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestMockAlerter(t *testing.T) {
	mock := NewMockAlerter()
	ctx := context.Background()

	// Initially empty
	if mock.Count() != 0 {
		t.Errorf("expected 0 alerts, got %d", mock.Count())
	}

	// Send alert
	err := mock.Alert(ctx, SeverityInfo, "test message", "key", "value")
	if err != nil {
		t.Fatalf("Alert() error = %v", err)
	}

	// Check count
	if mock.Count() != 1 {
		t.Errorf("expected 1 alert, got %d", mock.Count())
	}

	// Check last alert
	last := mock.LastAlert()
	if last == nil {
		t.Fatal("expected last alert, got nil")
	}
	if last.Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo, got %v", last.Severity)
	}
	if last.Message != "test message" {
		t.Errorf("expected 'test message', got %q", last.Message)
	}

	// Check contains
	if !mock.HasAlertContaining("test") {
		t.Error("expected to have alert containing 'test'")
	}
	if mock.HasAlertContaining("nonexistent") {
		t.Error("did not expect alert containing 'nonexistent'")
	}

	// Check severity
	if !mock.HasAlertWithSeverity(SeverityInfo) {
		t.Error("expected to have alert with SeverityInfo")
	}
	if mock.HasAlertWithSeverity(SeverityCritical) {
		t.Error("did not expect alert with SeverityCritical")
	}

	// Clear
	mock.Clear()
	if mock.Count() != 0 {
		t.Errorf("expected 0 alerts after clear, got %d", mock.Count())
	}
}

func TestConsoleAlerter(t *testing.T) {
	alerter := NewConsoleAlerter(nil)

	if alerter.Name() != "console" {
		t.Errorf("expected name 'console', got %q", alerter.Name())
	}

	// Should not error
	err := alerter.Alert(context.Background(), SeverityInfo, "test")
	if err != nil {
		t.Errorf("Alert() error = %v", err)
	}
}

func TestMultiAlerter(t *testing.T) {
	mock1 := NewMockAlerter()
	mock2 := NewMockAlerter()

	multi := NewMultiAlerter(nil, mock1, mock2)

	if multi.Name() != "multi" {
		t.Errorf("expected name 'multi', got %q", multi.Name())
	}

	// Send alert
	err := multi.Alert(context.Background(), SeverityWarning, "broadcast")
	if err != nil {
		t.Fatalf("Alert() error = %v", err)
	}

	// Both should receive
	if mock1.Count() != 1 {
		t.Errorf("mock1: expected 1 alert, got %d", mock1.Count())
	}
	if mock2.Count() != 1 {
		t.Errorf("mock2: expected 1 alert, got %d", mock2.Count())
	}

	// Add another alerter
	mock3 := NewMockAlerter()
	multi.AddAlerter(mock3)

	// Send another alert
	_ = multi.Alert(context.Background(), SeverityHigh, "another")

	if mock3.Count() != 1 {
		t.Errorf("mock3: expected 1 alert, got %d", mock3.Count())
	}
}

func TestMultiAlerter_AlertEvent(t *testing.T) {
	mock := NewMockAlerter()
	multi := NewMultiAlerter(nil, mock)

	err := multi.AlertEvent(context.Background(), EventKillSwitchActivated, "Kill switch triggered")
	if err != nil {
		t.Fatalf("AlertEvent() error = %v", err)
	}

	last := mock.LastAlert()
	if last == nil {
		t.Fatal("expected alert, got nil")
	}
	if last.Severity != SeverityCritical {
		t.Errorf("expected SeverityCritical, got %v", last.Severity)
	}
}
