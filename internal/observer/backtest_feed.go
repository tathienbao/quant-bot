package observer

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// BacktestFeed provides market data from CSV files for backtesting.
type BacktestFeed struct {
	filePath string
	symbol   string
	events   []types.MarketEvent
	loaded   bool
}

// NewBacktestFeed creates a new backtest feed from a CSV file.
// CSV format: timestamp,open,high,low,close,volume
// Timestamp format: 2006-01-02 15:04:05 or Unix timestamp
func NewBacktestFeed(filePath, symbol string) *BacktestFeed {
	return &BacktestFeed{
		filePath: filePath,
		symbol:   symbol,
	}
}

// Subscribe starts sending historical market events.
// The channel will close when all data has been sent or context is cancelled.
func (f *BacktestFeed) Subscribe(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	if !f.loaded {
		if err := f.load(); err != nil {
			return nil, err
		}
	}

	ch := make(chan types.MarketEvent, 100)

	go func() {
		defer close(ch)
		for _, event := range f.events {
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

// Close releases resources.
func (f *BacktestFeed) Close() error {
	f.events = nil
	f.loaded = false
	return nil
}

// Name returns the feed identifier.
func (f *BacktestFeed) Name() string {
	return "backtest"
}

// Load reads and parses the CSV file.
func (f *BacktestFeed) load() error {
	file, err := os.Open(f.filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	events, err := ParseCSV(file, f.symbol)
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}

	f.events = events
	f.loaded = true
	return nil
}

// EventCount returns the number of loaded events.
func (f *BacktestFeed) EventCount() int {
	return len(f.events)
}

// ParseCSV parses market data from a CSV reader.
// Supports formats:
// - timestamp,open,high,low,close,volume
// - timestamp,open,high,low,close,volume (with header row)
func ParseCSV(r io.Reader, symbol string) ([]types.MarketEvent, error) {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.TrimLeadingSpace = true

	var events []types.MarketEvent
	lineNum := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		lineNum++

		// Skip header row
		if lineNum == 1 && isHeader(record) {
			continue
		}

		if len(record) < 5 {
			continue // Skip invalid rows
		}

		event, err := parseRecord(record, symbol)
		if err != nil {
			// Skip invalid rows instead of failing
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

// parseRecord parses a single CSV record into a MarketEvent.
func parseRecord(record []string, symbol string) (types.MarketEvent, error) {
	var event types.MarketEvent
	event.Symbol = symbol

	// Parse timestamp
	ts, err := parseTimestamp(record[0])
	if err != nil {
		return event, fmt.Errorf("parse timestamp: %w", err)
	}
	event.Timestamp = ts

	// Parse OHLC
	event.Open, err = decimal.NewFromString(record[1])
	if err != nil {
		return event, fmt.Errorf("parse open: %w", err)
	}

	event.High, err = decimal.NewFromString(record[2])
	if err != nil {
		return event, fmt.Errorf("parse high: %w", err)
	}

	event.Low, err = decimal.NewFromString(record[3])
	if err != nil {
		return event, fmt.Errorf("parse low: %w", err)
	}

	event.Close, err = decimal.NewFromString(record[4])
	if err != nil {
		return event, fmt.Errorf("parse close: %w", err)
	}

	// Parse volume (optional)
	if len(record) > 5 {
		vol, err := strconv.ParseInt(record[5], 10, 64)
		if err == nil {
			event.Volume = vol
		}
	}

	return event, nil
}

// parseTimestamp tries multiple timestamp formats.
func parseTimestamp(s string) (time.Time, error) {
	// Try Unix timestamp first
	if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(unix, 0), nil
	}

	// Try common date formats
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04",
		"2006-01-02",
		"01/02/2006 15:04:05",
		"01/02/2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unknown timestamp format: %s", s)
}

// isHeader checks if a record looks like a header row.
func isHeader(record []string) bool {
	if len(record) == 0 {
		return false
	}
	// Common header names
	headers := []string{"timestamp", "time", "date", "datetime", "open", "high", "low", "close"}
	first := record[0]
	for _, h := range headers {
		if first == h {
			return true
		}
	}
	return false
}

// MemoryFeed provides market data from an in-memory slice.
// Useful for testing.
type MemoryFeed struct {
	events []types.MarketEvent
	symbol string
}

// NewMemoryFeed creates a feed from pre-loaded events.
func NewMemoryFeed(events []types.MarketEvent, symbol string) *MemoryFeed {
	return &MemoryFeed{
		events: events,
		symbol: symbol,
	}
}

// Subscribe starts sending events from memory.
func (f *MemoryFeed) Subscribe(ctx context.Context, symbol string) (<-chan types.MarketEvent, error) {
	ch := make(chan types.MarketEvent, len(f.events))

	go func() {
		defer close(ch)
		for _, event := range f.events {
			if event.Symbol != symbol && f.symbol != symbol {
				continue
			}
			// Override symbol if feed has a fixed symbol
			if f.symbol != "" {
				event.Symbol = f.symbol
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

// Close is a no-op for memory feed.
func (f *MemoryFeed) Close() error {
	return nil
}

// Name returns the feed identifier.
func (f *MemoryFeed) Name() string {
	return "memory"
}

// AddEvent adds an event to the feed.
func (f *MemoryFeed) AddEvent(event types.MarketEvent) {
	f.events = append(f.events, event)
}
