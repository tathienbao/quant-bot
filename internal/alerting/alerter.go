// Package alerting provides notification capabilities for the trading bot.
package alerting

import (
	"context"
	"fmt"
)

// Severity represents the alert severity level.
type Severity int

const (
	// SeverityInfo is for informational messages.
	SeverityInfo Severity = iota
	// SeverityWarning is for warning messages.
	SeverityWarning
	// SeverityHigh is for high priority alerts.
	SeverityHigh
	// SeverityCritical is for critical alerts requiring immediate attention.
	SeverityCritical
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARNING"
	case SeverityHigh:
		return "HIGH"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// Emoji returns an emoji for the severity level.
func (s Severity) Emoji() string {
	switch s {
	case SeverityInfo:
		return "ℹ️"
	case SeverityWarning:
		return "⚠️"
	case SeverityHigh:
		return "🔴"
	case SeverityCritical:
		return "🚨"
	default:
		return "❓"
	}
}

// Alerter defines the interface for sending alerts.
type Alerter interface {
	// Alert sends an alert with the given severity and message.
	Alert(ctx context.Context, severity Severity, message string, fields ...any) error
	// Name returns the name of the alerter.
	Name() string
}

// Field represents a key-value pair for structured alert data.
type Field struct {
	Key   string
	Value any
}

// FormatFields converts variadic fields to a formatted string.
func FormatFields(fields ...any) string {
	if len(fields) == 0 {
		return ""
	}

	result := ""
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		value := fields[i+1]
		if result != "" {
			result += "\n"
		}
		result += fmt.Sprintf("• %s: %v", key, value)
	}
	return result
}

// AlertEvent represents a pre-defined alert event type.
type AlertEvent string

const (
	// EventKillSwitchActivated is sent when kill switch is triggered.
	EventKillSwitchActivated AlertEvent = "kill_switch_activated"
	// EventSafeModeEntered is sent when safe mode is entered.
	EventSafeModeEntered AlertEvent = "safe_mode_entered"
	// EventSafeModeExited is sent when safe mode is exited.
	EventSafeModeExited AlertEvent = "safe_mode_exited"
	// EventOrderFilled is sent when an order is filled.
	EventOrderFilled AlertEvent = "order_filled"
	// EventOrderRejected is sent when an order is rejected.
	EventOrderRejected AlertEvent = "order_rejected"
	// EventPositionOpened is sent when a position is opened.
	EventPositionOpened AlertEvent = "position_opened"
	// EventPositionClosed is sent when a position is closed.
	EventPositionClosed AlertEvent = "position_closed"
	// EventDailySummary is sent for daily trading summary.
	EventDailySummary AlertEvent = "daily_summary"
	// EventConnectionLost is sent when connection is lost.
	EventConnectionLost AlertEvent = "connection_lost"
	// EventConnectionRestored is sent when connection is restored.
	EventConnectionRestored AlertEvent = "connection_restored"
	// EventBotStarted is sent when bot starts.
	EventBotStarted AlertEvent = "bot_started"
	// EventBotStopped is sent when bot stops.
	EventBotStopped AlertEvent = "bot_stopped"
)

// EventSeverity returns the default severity for an event.
func EventSeverity(event AlertEvent) Severity {
	switch event {
	case EventKillSwitchActivated:
		return SeverityCritical
	case EventSafeModeEntered, EventSafeModeExited:
		return SeverityHigh
	case EventOrderRejected, EventConnectionLost:
		return SeverityWarning
	case EventOrderFilled, EventPositionOpened, EventPositionClosed:
		return SeverityInfo
	case EventDailySummary, EventBotStarted, EventBotStopped, EventConnectionRestored:
		return SeverityInfo
	default:
		return SeverityInfo
	}
}
