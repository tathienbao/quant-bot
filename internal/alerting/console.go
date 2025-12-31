package alerting

import (
	"context"
	"log/slog"
)

// ConsoleAlerter logs alerts to the console using slog.
// Useful for development and testing.
type ConsoleAlerter struct {
	logger *slog.Logger
}

// NewConsoleAlerter creates a new console alerter.
func NewConsoleAlerter(logger *slog.Logger) *ConsoleAlerter {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConsoleAlerter{logger: logger}
}

// Name returns the name of the alerter.
func (c *ConsoleAlerter) Name() string {
	return "console"
}

// Alert logs an alert to the console.
func (c *ConsoleAlerter) Alert(ctx context.Context, severity Severity, message string, fields ...any) error {
	// Convert fields to slog attrs
	attrs := make([]any, 0, len(fields)+2)
	attrs = append(attrs, "severity", severity.String())
	attrs = append(attrs, fields...)

	switch severity {
	case SeverityCritical:
		c.logger.Error("[ALERT] "+message, attrs...)
	case SeverityHigh:
		c.logger.Warn("[ALERT] "+message, attrs...)
	case SeverityWarning:
		c.logger.Warn("[ALERT] "+message, attrs...)
	default:
		c.logger.Info("[ALERT] "+message, attrs...)
	}

	return nil
}
