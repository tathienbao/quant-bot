package observer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/types"
)

// TestNewBacktestFeed tests feed constructor.
func TestNewBacktestFeed(t *testing.T) {
	feed := NewBacktestFeed("/path/to/file.csv", "MES")

	if feed == nil {
		t.Fatal("expected feed to be created")
	}

	if feed.filePath != "/path/to/file.csv" {
		t.Errorf("expected filePath to be set")
	}

	if feed.symbol != "MES" {
		t.Errorf("expected symbol MES")
	}
}

// TestBacktestFeed_Name tests feed name.
func TestBacktestFeed_Name(t *testing.T) {
	feed := NewBacktestFeed("file.csv", "MES")
	if feed.Name() != "backtest" {
		t.Errorf("expected name 'backtest', got '%s'", feed.Name())
	}
}

// TestBacktestFeed_Subscribe tests event streaming.
func TestBacktestFeed_Subscribe(t *testing.T) {
	// Create temp CSV file
	csvData := `timestamp,open,high,low,close,volume
2024-01-01 09:30:00,5000,5010,4990,5005,1000
2024-01-01 09:35:00,5005,5015,5000,5010,1200
2024-01-01 09:40:00,5010,5020,5005,5015,1100
`
	tmpFile := createTempCSV(t, csvData)
	defer os.Remove(tmpFile)

	feed := NewBacktestFeed(tmpFile, "MES")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := feed.Subscribe(ctx, "MES")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Receive events
	events := make([]types.MarketEvent, 0)
	for event := range ch {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Verify first event
	if events[0].Symbol != "MES" {
		t.Error("expected symbol MES")
	}
	if !events[0].Open.Equal(decimal.NewFromInt(5000)) {
		t.Errorf("expected open 5000, got %s", events[0].Open.String())
	}
}

// TestBacktestFeed_Subscribe_FileNotFound tests error handling.
func TestBacktestFeed_Subscribe_FileNotFound(t *testing.T) {
	feed := NewBacktestFeed("/nonexistent/file.csv", "MES")

	ctx := context.Background()
	_, err := feed.Subscribe(ctx, "MES")

	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestBacktestFeed_Close tests resource cleanup.
func TestBacktestFeed_Close(t *testing.T) {
	feed := NewBacktestFeed("file.csv", "MES")
	feed.loaded = true
	feed.events = make([]types.MarketEvent, 10)

	err := feed.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if feed.loaded {
		t.Error("expected loaded to be false after close")
	}

	if len(feed.events) != 0 {
		t.Error("expected events to be cleared")
	}
}

// TestBacktestFeed_EventCount tests event count.
func TestBacktestFeed_EventCount(t *testing.T) {
	feed := NewBacktestFeed("file.csv", "MES")

	if feed.EventCount() != 0 {
		t.Error("expected 0 events initially")
	}

	feed.events = make([]types.MarketEvent, 5)
	if feed.EventCount() != 5 {
		t.Errorf("expected 5 events, got %d", feed.EventCount())
	}
}

// TestParseCSV_ValidData tests CSV parsing (MD-01 related).
func TestParseCSV_ValidData(t *testing.T) {
	csvData := `timestamp,open,high,low,close,volume
2024-01-01 09:30:00,5000.25,5010.50,4990.00,5005.75,1000
`
	reader := strings.NewReader(csvData)
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Symbol != "MES" {
		t.Error("expected symbol MES")
	}

	expected := decimal.RequireFromString("5000.25")
	if !event.Open.Equal(expected) {
		t.Errorf("expected open %s, got %s", expected.String(), event.Open.String())
	}
}

// TestParseCSV_UnixTimestamp tests Unix timestamp parsing.
func TestParseCSV_UnixTimestamp(t *testing.T) {
	csvData := `1704110400,5000,5010,4990,5005,1000
`
	reader := strings.NewReader(csvData)
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Verify timestamp was parsed
	if events[0].Timestamp.IsZero() {
		t.Error("expected valid timestamp")
	}
}

// TestParseCSV_InvalidData tests invalid OHLC handling (MD-01).
func TestParseCSV_InvalidData(t *testing.T) {
	// CSV with invalid numbers - should skip invalid rows
	csvData := `timestamp,open,high,low,close,volume
2024-01-01 09:30:00,invalid,5010,4990,5005,1000
2024-01-01 09:35:00,5005,5015,5000,5010,1200
`
	reader := strings.NewReader(csvData)
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should skip invalid row
	if len(events) != 1 {
		t.Errorf("expected 1 valid event (skipping invalid), got %d", len(events))
	}
}

// TestParseCSV_NoHeader tests parsing without header.
func TestParseCSV_NoHeader(t *testing.T) {
	csvData := `2024-01-01 09:30:00,5000,5010,4990,5005,1000
2024-01-01 09:35:00,5005,5015,5000,5010,1200
`
	reader := strings.NewReader(csvData)
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

// TestParseTimestamp_MultipleFormats tests timestamp format support (TIME-03).
func TestParseTimestamp_MultipleFormats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Unix timestamp", "1704110400", false},
		{"ISO datetime", "2024-01-01 09:30:00", false},
		{"ISO with T", "2024-01-01T09:30:00", false},
		{"ISO with Z", "2024-01-01T09:30:00Z", false},
		{"Date only", "2024-01-01", false},
		{"US format", "01/02/2024 09:30:00", false},
		{"Invalid", "not-a-date", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := parseTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if ts.IsZero() {
				t.Error("expected non-zero timestamp")
			}
		})
	}
}

// TestParseCSV_EmptyFile tests empty file handling.
func TestParseCSV_EmptyFile(t *testing.T) {
	reader := strings.NewReader("")
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// TestParseCSV_ShortRow tests rows with missing columns.
func TestParseCSV_ShortRow(t *testing.T) {
	// CSV with consistent short rows (4 fields instead of 6)
	csvData := `2024-01-01 09:30:00,5000,5010,4990
2024-01-01 09:35:00,5005,5015,5000
`
	reader := strings.NewReader(csvData)
	events, err := ParseCSV(reader, "MES")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should skip rows with <5 columns
	if len(events) != 0 {
		t.Errorf("expected 0 events (all rows short), got %d", len(events))
	}
}

// TestBacktestFeed_Subscribe_ContextCancelled tests cancellation.
func TestBacktestFeed_Subscribe_ContextCancelled(t *testing.T) {
	// Create CSV with many rows
	var csvBuilder strings.Builder
	csvBuilder.WriteString("timestamp,open,high,low,close,volume\n")
	for i := 0; i < 100; i++ {
		csvBuilder.WriteString("2024-01-01 09:30:00,5000,5010,4990,5005,1000\n")
	}

	tmpFile := createTempCSV(t, csvBuilder.String())
	defer os.Remove(tmpFile)

	feed := NewBacktestFeed(tmpFile, "MES")

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := feed.Subscribe(ctx, "MES")
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Receive a few events then cancel
	received := 0
	for range ch {
		received++
		if received >= 5 {
			cancel()
			break
		}
	}

	// Drain
	for range ch {
	}

	if received < 5 {
		t.Errorf("expected at least 5 events, got %d", received)
	}
}

// Helper to create temp CSV file.
func createTempCSV(t *testing.T, content string) string {
	t.Helper()

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_data.csv")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	return tmpFile
}

