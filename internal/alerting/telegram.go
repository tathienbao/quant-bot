package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelegramConfig holds configuration for Telegram alerter.
type TelegramConfig struct {
	BotToken string
	ChatID   string
	Timeout  time.Duration
}

// TelegramAlerter sends alerts via Telegram.
type TelegramAlerter struct {
	cfg    TelegramConfig
	client *http.Client
}

// NewTelegramAlerter creates a new Telegram alerter.
func NewTelegramAlerter(cfg TelegramConfig) *TelegramAlerter {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &TelegramAlerter{
		cfg: cfg,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the name of the alerter.
func (t *TelegramAlerter) Name() string {
	return "telegram"
}

// telegramMessage represents the Telegram API message format.
type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// telegramResponse represents the Telegram API response.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// Alert sends an alert via Telegram.
func (t *TelegramAlerter) Alert(ctx context.Context, severity Severity, message string, fields ...any) error {
	// Format the message
	text := t.formatMessage(severity, message, fields...)

	// Create request body
	msg := telegramMessage{
		ChatID:    t.cfg.ChatID,
		Text:      text,
		ParseMode: "HTML",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Create request
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Parse response
	var telegramResp telegramResponse
	if err := json.Unmarshal(respBody, &telegramResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("telegram API error: %s", telegramResp.Description)
	}

	return nil
}

// formatMessage formats the alert message for Telegram.
func (t *TelegramAlerter) formatMessage(severity Severity, message string, fields ...any) string {
	// Build message with emoji and severity
	text := fmt.Sprintf("%s <b>[%s]</b>\n%s", severity.Emoji(), severity.String(), message)

	// Add fields if present
	if len(fields) > 0 {
		fieldsStr := FormatFields(fields...)
		if fieldsStr != "" {
			text += "\n\n<b>Details:</b>\n" + fieldsStr
		}
	}

	// Add timestamp
	text += fmt.Sprintf("\n\n<i>%s</i>", time.Now().Format("2006-01-02 15:04:05 MST"))

	return text
}

// SendDailySummary sends a formatted daily trading summary.
func (t *TelegramAlerter) SendDailySummary(ctx context.Context, summary DailySummary) error {
	text := t.formatDailySummary(summary)

	msg := telegramMessage{
		ChatID:    t.cfg.ChatID,
		Text:      text,
		ParseMode: "HTML",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var telegramResp telegramResponse
	if err := json.Unmarshal(respBody, &telegramResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("telegram API error: %s", telegramResp.Description)
	}

	return nil
}

// formatDailySummary formats a daily summary for Telegram.
func (t *TelegramAlerter) formatDailySummary(s DailySummary) string {
	// Determine emoji based on P&L
	plEmoji := "📈"
	if s.TotalPL.IsNegative() {
		plEmoji = "📉"
	}

	text := fmt.Sprintf(`%s <b>Daily Trading Summary</b>
<b>Date:</b> %s

<b>Performance:</b>
• Starting Equity: $%s
• Ending Equity: $%s
• Daily P/L: $%s (%s%%)
• High Water Mark: $%s
• Drawdown: %s%%

<b>Trades:</b>
• Total: %d
• Wins: %d | Losses: %d
• Win Rate: %s%%

<b>Status:</b>
• Safe Mode: %s
• Open Positions: %d`,
		plEmoji,
		s.Date.Format("2006-01-02"),
		s.StartingEquity.StringFixed(2),
		s.EndingEquity.StringFixed(2),
		s.TotalPL.StringFixed(2),
		s.ReturnPct.StringFixed(2),
		s.HighWaterMark.StringFixed(2),
		s.Drawdown.StringFixed(2),
		s.TotalTrades,
		s.WinningTrades,
		s.LosingTrades,
		s.WinRate.StringFixed(1),
		boolToStatus(s.SafeModeActive),
		s.OpenPositions,
	)

	return text
}

func boolToStatus(b bool) string {
	if b {
		return "🔴 Active"
	}
	return "🟢 Inactive"
}
