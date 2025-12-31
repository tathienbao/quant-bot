package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/shopspring/decimal"
	"golang.org/x/term"
)

// ANSI escape codes
const (
	ClearLine    = "\033[2K"
	MoveToStart  = "\r"
	MoveUp       = "\033[%dA"
	HideCursor   = "\033[?25l"
	ShowCursor   = "\033[?25h"
	ColorReset   = "\033[0m"
	ColorGreen   = "\033[32m"
	ColorRed     = "\033[31m"
	ColorYellow  = "\033[33m"
	ColorCyan    = "\033[36m"
	ColorDim     = "\033[2m"
	ColorBold    = "\033[1m"
)

// Candle represents OHLC data for one bar
type Candle struct {
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume int64
}

// BacktestUI handles terminal display for backtesting
type BacktestUI struct {
	candles    []Candle
	maxCandles int
	chartHeight int

	// Stats
	currentBar   int
	totalBars    int
	equity       decimal.Decimal
	startEquity  decimal.Decimal
	trades       int
	winRate      decimal.Decimal
	lastSignal   string

	// Terminal
	width  int
	height int

	// Track lines printed for cleanup
	linesPrinted int
}

// NewBacktestUI creates a new backtest UI
func NewBacktestUI(totalBars int, startEquity decimal.Decimal) *BacktestUI {
	width, height := getTerminalSize()

	// Reserve space for stats (3 lines) and padding
	chartHeight := 12
	maxCandles := width - 20 // Leave room for price axis
	if maxCandles < 20 {
		maxCandles = 20
	}
	if maxCandles > 100 {
		maxCandles = 100
	}

	return &BacktestUI{
		candles:     make([]Candle, 0, maxCandles),
		maxCandles:  maxCandles,
		chartHeight: chartHeight,
		totalBars:   totalBars,
		startEquity: startEquity,
		equity:      startEquity,
		width:       width,
		height:      height,
	}
}

// Start initializes the UI
func (ui *BacktestUI) Start() {
	fmt.Print(HideCursor)
	fmt.Println() // Initial newline
}

// Stop cleans up the UI
func (ui *BacktestUI) Stop() {
	fmt.Print(ShowCursor)
	fmt.Println() // Final newline
}

// AddCandle adds a new candle and updates display
func (ui *BacktestUI) AddCandle(c Candle) {
	ui.candles = append(ui.candles, c)
	if len(ui.candles) > ui.maxCandles {
		ui.candles = ui.candles[1:]
	}
	ui.currentBar++
}

// UpdateStats updates trading statistics
func (ui *BacktestUI) UpdateStats(equity decimal.Decimal, trades int, winRate decimal.Decimal, signal string) {
	ui.equity = equity
	ui.trades = trades
	ui.winRate = winRate
	if signal != "" {
		ui.lastSignal = signal
	}
}

// Render draws the current state
func (ui *BacktestUI) Render() {
	// Move cursor up to overwrite previous frame
	if ui.linesPrinted > 0 {
		fmt.Printf("\033[%dA", ui.linesPrinted)
	}

	var lines []string

	// Progress bar
	progress := float64(ui.currentBar) / float64(ui.totalBars)
	progressWidth := ui.width - 30
	if progressWidth < 20 {
		progressWidth = 20
	}
	filled := int(progress * float64(progressWidth))
	progressBar := strings.Repeat("█", filled) + strings.Repeat("░", progressWidth-filled)
	lines = append(lines, fmt.Sprintf("%s%s %.1f%% [%d/%d]%s",
		ColorCyan, progressBar, progress*100, ui.currentBar, ui.totalBars, ColorReset))

	// Chart
	chartLines := ui.renderChart()
	lines = append(lines, chartLines...)

	// Stats line
	pnlPct := decimal.Zero
	if !ui.startEquity.IsZero() {
		pnlPct = ui.equity.Sub(ui.startEquity).Div(ui.startEquity).Mul(decimal.NewFromInt(100))
	}
	pnlColor := ColorGreen
	if pnlPct.LessThan(decimal.Zero) {
		pnlColor = ColorRed
	}

	statsLine := fmt.Sprintf("%sEquity:%s $%.0f (%s%+.2f%%%s) │ %sTrades:%s %d │ %sWin:%s %.1f%% │ %sSignal:%s %s",
		ColorBold, ColorReset, ui.equity.InexactFloat64(),
		pnlColor, pnlPct.InexactFloat64(), ColorReset,
		ColorBold, ColorReset, ui.trades,
		ColorBold, ColorReset, ui.winRate.InexactFloat64(),
		ColorBold, ColorReset, ui.lastSignal)
	lines = append(lines, statsLine)

	// Print all lines
	for _, line := range lines {
		fmt.Print(ClearLine)
		fmt.Println(line)
	}

	ui.linesPrinted = len(lines)
}

// renderChart creates ASCII candlestick chart
func (ui *BacktestUI) renderChart() []string {
	if len(ui.candles) < 2 {
		lines := make([]string, ui.chartHeight)
		for i := range lines {
			lines[i] = ColorDim + "│" + ColorReset
		}
		return lines
	}

	// Find price range
	minPrice := ui.candles[0].Low
	maxPrice := ui.candles[0].High
	for _, c := range ui.candles {
		if c.Low.LessThan(minPrice) {
			minPrice = c.Low
		}
		if c.High.GreaterThan(maxPrice) {
			maxPrice = c.High
		}
	}

	// Add padding to price range
	priceRange := maxPrice.Sub(minPrice)
	if priceRange.IsZero() {
		priceRange = decimal.NewFromInt(1)
	}
	padding := priceRange.Mul(decimal.RequireFromString("0.05"))
	minPrice = minPrice.Sub(padding)
	maxPrice = maxPrice.Add(padding)
	priceRange = maxPrice.Sub(minPrice)

	// Build chart matrix
	height := ui.chartHeight
	width := len(ui.candles)
	chart := make([][]rune, height)
	colors := make([][]string, height)
	for i := range chart {
		chart[i] = make([]rune, width)
		colors[i] = make([]string, width)
		for j := range chart[i] {
			chart[i][j] = ' '
			colors[i][j] = ColorReset
		}
	}

	// Draw candles
	for x, candle := range ui.candles {
		isGreen := candle.Close.GreaterThanOrEqual(candle.Open)
		color := ColorRed
		if isGreen {
			color = ColorGreen
		}

		// Convert prices to y coordinates (0 = top, height-1 = bottom)
		highY := priceToY(candle.High, minPrice, priceRange, height)
		lowY := priceToY(candle.Low, minPrice, priceRange, height)
		openY := priceToY(candle.Open, minPrice, priceRange, height)
		closeY := priceToY(candle.Close, minPrice, priceRange, height)

		// Body top and bottom
		bodyTop := openY
		bodyBottom := closeY
		if closeY < openY {
			bodyTop = closeY
			bodyBottom = openY
		}

		// Draw wick
		for y := highY; y <= lowY; y++ {
			if y >= 0 && y < height {
				chart[y][x] = '│'
				colors[y][x] = color
			}
		}

		// Draw body
		for y := bodyTop; y <= bodyBottom; y++ {
			if y >= 0 && y < height {
				if isGreen {
					chart[y][x] = '█'
				} else {
					chart[y][x] = '█'
				}
				colors[y][x] = color
			}
		}
	}

	// Convert to strings with price axis
	lines := make([]string, height)
	for y := 0; y < height; y++ {
		var sb strings.Builder

		// Price label (every 3 rows)
		if y%(height/4) == 0 {
			price := yToPrice(y, minPrice, priceRange, height)
			sb.WriteString(fmt.Sprintf("%s%7.1f%s │", ColorDim, price.InexactFloat64(), ColorReset))
		} else {
			sb.WriteString(fmt.Sprintf("%s        │%s", ColorDim, ColorReset))
		}

		// Chart content
		for x := 0; x < width; x++ {
			sb.WriteString(colors[y][x])
			sb.WriteRune(chart[y][x])
		}
		sb.WriteString(ColorReset)

		lines[y] = sb.String()
	}

	// Add bottom axis
	axisLine := strings.Repeat("─", width)
	lines = append(lines, fmt.Sprintf("%s        └%s%s", ColorDim, axisLine, ColorReset))

	return lines
}

// priceToY converts a price to y coordinate
func priceToY(price, minPrice, priceRange decimal.Decimal, height int) int {
	if priceRange.IsZero() {
		return height / 2
	}
	normalized := price.Sub(minPrice).Div(priceRange)
	y := decimal.NewFromInt(int64(height - 1)).Sub(normalized.Mul(decimal.NewFromInt(int64(height - 1))))
	return int(y.IntPart())
}

// yToPrice converts y coordinate back to price
func yToPrice(y int, minPrice, priceRange decimal.Decimal, height int) decimal.Decimal {
	normalized := decimal.NewFromInt(int64(height - 1 - y)).Div(decimal.NewFromInt(int64(height - 1)))
	return minPrice.Add(priceRange.Mul(normalized))
}

// getTerminalSize returns terminal dimensions
func getTerminalSize() (width, height int) {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80, 24 // Default
	}
	return width, height
}

// ProgressLine prints a single updating progress line
func ProgressLine(current, total int, message string) {
	progress := float64(current) / float64(total) * 100
	fmt.Printf("%s%s[%d/%d] %.1f%% - %s", ClearLine, MoveToStart, current, total, progress, message)
}
