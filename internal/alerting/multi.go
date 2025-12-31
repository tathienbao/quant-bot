package alerting

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// MultiAlerter sends alerts to multiple channels.
type MultiAlerter struct {
	mu       sync.RWMutex
	alerters []Alerter
	logger   *slog.Logger
}

// NewMultiAlerter creates a new multi-channel alerter.
func NewMultiAlerter(logger *slog.Logger, alerters ...Alerter) *MultiAlerter {
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiAlerter{
		alerters: alerters,
		logger:   logger,
	}
}

// Name returns the name of the alerter.
func (m *MultiAlerter) Name() string {
	return "multi"
}

// AddAlerter adds a new alerter to the multi-alerter.
func (m *MultiAlerter) AddAlerter(alerter Alerter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerters = append(m.alerters, alerter)
}

// Alert sends an alert to all configured channels.
// Returns an error if any channel fails (errors are joined).
func (m *MultiAlerter) Alert(ctx context.Context, severity Severity, message string, fields ...any) error {
	m.mu.RLock()
	alerters := make([]Alerter, len(m.alerters))
	copy(alerters, m.alerters)
	m.mu.RUnlock()

	if len(alerters) == 0 {
		return nil
	}

	var errs []error
	var wg sync.WaitGroup

	errCh := make(chan error, len(alerters))

	for _, alerter := range alerters {
		wg.Add(1)
		go func(a Alerter) {
			defer wg.Done()
			if err := a.Alert(ctx, severity, message, fields...); err != nil {
				m.logger.Error("alerter failed",
					"alerter", a.Name(),
					"severity", severity.String(),
					"error", err,
				)
				errCh <- err
			}
		}(alerter)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// AlertEvent sends an alert for a predefined event type.
func (m *MultiAlerter) AlertEvent(ctx context.Context, event AlertEvent, message string, fields ...any) error {
	severity := EventSeverity(event)
	return m.Alert(ctx, severity, message, fields...)
}
