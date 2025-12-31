// Package main is the entry point for the quant trading bot.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/alerting"
	"github.com/tathienbao/quant-bot/internal/backtest"
	"github.com/tathienbao/quant-bot/internal/broker/paper"
	"github.com/tathienbao/quant-bot/internal/config"
	"github.com/tathienbao/quant-bot/internal/engine"
	"github.com/tathienbao/quant-bot/internal/execution"
	"github.com/tathienbao/quant-bot/internal/metrics"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/persistence"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/strategy"
	"github.com/tathienbao/quant-bot/internal/ui"
)

// Version information (set by build flags).
var (
	Version   = "1.4.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Parse command
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		printUsage()
	case "backtest":
		cmdBacktest(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	case "validate":
		cmdValidate(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Quant Trading Bot - Risk-First MES/MGC Futures Trading

Usage:
  quant-bot <command> [options]

Commands:
  run        Start the trading bot (live or paper)
  backtest   Run a backtest simulation
  validate   Validate configuration file
  version    Show version information
  help       Show this help message

Examples:
  quant-bot run --config config.yaml
  quant-bot backtest --config config.yaml --data data/MES_5m.csv
  quant-bot validate --config config.yaml

Use "quant-bot <command> --help" for more information about a command.`)
}

// strategyOption represents a strategy choice in the menu.
type strategyOption struct {
	Name        string
	Description string
	Return      string
	WinRate     string
	Recommended bool
}

// selectStrategy shows an interactive menu to select a strategy.
func selectStrategy() string {
	options := []strategyOption{
		{
			Name:        "grid",
			Description: "Grid/Rebound - High Frequency (max return)",
			Return:      "+51.94%",
			WinRate:     "91.05%",
			Recommended: true,
		},
		{
			Name:        "grid-conservative",
			Description: "Grid/Rebound - Conservative (low risk)",
			Return:      "+33.54%",
			WinRate:     "85.41%",
			Recommended: false,
		},
		{
			Name:        "breakout",
			Description: "Range Breakout (không khuyến nghị)",
			Return:      "-11.59%",
			WinRate:     "0%",
			Recommended: false,
		},
		{
			Name:        "meanrev",
			Description: "Mean Reversion (không khuyến nghị)",
			Return:      "-3.62%",
			WinRate:     "20%",
			Recommended: false,
		},
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Name | cyan }} - {{ .Description }} (Return: {{ .Return }}, WR: {{ .WinRate }}){{ if .Recommended }} ⭐{{ end }}",
		Inactive: "  {{ .Name | white }} - {{ .Description }} (Return: {{ .Return }}, WR: {{ .WinRate }}){{ if .Recommended }} ⭐{{ end }}",
		Selected: "✔ Strategy: {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Chọn Strategy (↑↓ để di chuyển, Enter để chọn)",
		Items:     options,
		Templates: templates,
		Size:      6,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Selection cancelled\n")
		os.Exit(1)
	}

	return options[idx].Name
}

// selectDataFile shows an interactive menu to select a data file.
func selectDataFile() string {
	// Find CSV files in data directory
	files, err := filepath.Glob("data/*.csv")
	if err != nil || len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No CSV files found in data/ directory\n")
		os.Exit(1)
	}

	type fileOption struct {
		Path string
		Name string
	}

	options := make([]fileOption, len(files))
	for i, f := range files {
		options[i] = fileOption{
			Path: f,
			Name: filepath.Base(f),
		}
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Name | cyan }}",
		Inactive: "  {{ .Name | white }}",
		Selected: "✔ Data file: {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Chọn Data File (↑↓ để di chuyển, Enter để chọn)",
		Items:     options,
		Templates: templates,
		Size:      6,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Selection cancelled\n")
		os.Exit(1)
	}

	return options[idx].Path
}

func cmdVersion() {
	fmt.Printf("quant-bot version %s\n", Version)
	fmt.Printf("  Build time: %s\n", BuildTime)
	fmt.Printf("  Git commit: %s\n", GitCommit)
}

func cmdValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "Path to configuration file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration is valid!")
	fmt.Printf("  Starting equity: $%.2f\n", cfg.Account.StartingEquity)
	fmt.Printf("  Primary instrument: %s\n", cfg.Market.InstrumentPrimary)
	fmt.Printf("  Max drawdown: %.1f%%\n", cfg.Account.MaxGlobalDrawdownPct*100)
	fmt.Printf("  Risk per trade: %.1f%%\n", cfg.Account.RiskPerTradePct*100)
}

func cmdBacktest(args []string) {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "Path to configuration file")
	dataPath := fs.String("data", "", "Path to CSV data file (interactive if empty)")
	strategyName := fs.String("strategy", "", "Strategy (interactive if empty)")
	verbose := fs.Bool("verbose", false, "Verbose output")
	interactive := fs.Bool("i", false, "Force interactive mode")
	showUI := fs.Bool("ui", true, "Show live chart UI (default: true)")
	fs.Parse(args)

	// Interactive mode for data file
	if *dataPath == "" || *interactive {
		*dataPath = selectDataFile()
	}

	// Interactive mode for strategy
	if *strategyName == "" || *interactive {
		*strategyName = selectStrategy()
	}

	// Setup logging - suppress if UI enabled
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	if *showUI {
		logLevel = slog.LevelError // Suppress logs when UI active
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Count total bars for progress
	totalBars := countCSVLines(*dataPath)

	// Create feed
	feed := observer.NewBacktestFeed(*dataPath, cfg.Market.InstrumentPrimary)

	// Create calculator
	calculator := observer.NewCalculator(observer.CalculatorConfig{
		ATRPeriod:    cfg.Risk.VolatilityLookbackBars,
		StdDevPeriod: 20,
	})

	// Create strategy
	var strat strategy.Strategy
	switch *strategyName {
	case "breakout":
		strat = strategy.NewBreakout(strategy.BreakoutConfig{
			LookbackBars:   cfg.Risk.VolatilityLookbackBars,
			ATRMultiplier:  decimal.NewFromFloat(cfg.Risk.StopLossATRMultiple),
			BreakoutBuffer: decimal.Zero,
		})
	case "meanrev":
		strat = strategy.NewMeanReversion(strategy.MeanRevConfig{
			SMAPeriod:     20,
			StdDevPeriod:  20,
			EntryStdDev:   decimal.RequireFromString("2.0"),
			ATRMultiplier: decimal.NewFromFloat(cfg.Risk.StopLossATRMultiple),
		})
	case "grid":
		strat = strategy.NewGrid(strategy.OriginalGridConfig())
	case "grid-conservative":
		strat = strategy.NewGrid(strategy.ConservativeGridConfig())
	default:
		fmt.Fprintf(os.Stderr, "unknown strategy: %s\n", *strategyName)
		os.Exit(1)
	}

	// Create execution config
	execCfg := execution.SimulatedConfig{
		SlippageTicks:     cfg.Backtest.SlippageTicks,
		CommissionPerSide: decimal.NewFromFloat(cfg.Backtest.CommissionPerContract / 2),
	}

	// Create runner
	runner := backtest.NewRunner(
		backtest.Config{InitialEquity: cfg.StartingEquityDecimal()},
		feed,
		calculator,
		strat,
		cfg.ToRiskConfig(),
		execCfg,
	)
	runner.SetTotalBars(totalBars)

	// Setup UI if enabled
	var backtestUI *ui.BacktestUI
	if *showUI {
		backtestUI = ui.NewBacktestUI(totalBars, cfg.StartingEquityDecimal())
		backtestUI.Start()
		defer backtestUI.Stop()

		// Set progress callback
		lastRender := time.Now()
		runner.SetProgressCallback(func(update backtest.ProgressUpdate) {
			// Add candle
			backtestUI.AddCandle(ui.Candle{
				Open:  update.Event.Open,
				High:  update.Event.High,
				Low:   update.Event.Low,
				Close: update.Event.Close,
			})

			// Update stats
			backtestUI.UpdateStats(update.Equity, update.Trades, update.WinRate, update.LastSignal)

			// Render at most 30 FPS
			if time.Since(lastRender) > 33*time.Millisecond {
				backtestUI.Render()
				lastRender = time.Now()
			}
		})
	} else {
		slog.Info("starting backtest",
			"data", *dataPath,
			"strategy", *strategyName,
			"equity", cfg.Account.StartingEquity,
		)
	}

	// Run backtest
	ctx := context.Background()
	result, err := runner.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backtest failed: %v\n", err)
		os.Exit(1)
	}

	// Final render
	if backtestUI != nil {
		backtestUI.Render()
	}

	// Print results
	printBacktestResults(result, cfg.Account.StartingEquity)

	// Calculate metrics
	metrics := backtest.NewMetrics(result, decimal.Zero)
	printMetrics(metrics)
}

// countCSVLines counts the number of data lines in a CSV file
func countCSVLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
	}
	// Subtract 1 for header
	if count > 0 {
		count--
	}
	return count
}

func printBacktestResults(result *backtest.Result, startingEquity float64) {
	fmt.Println("\n=== BACKTEST RESULTS ===")
	fmt.Printf("Starting Equity:  $%.2f\n", result.StartEquity.InexactFloat64())
	fmt.Printf("Ending Equity:    $%.2f\n", result.EndEquity.InexactFloat64())
	fmt.Printf("Total Return:     %.2f%%\n", result.TotalReturn.Mul(decimal.NewFromInt(100)).InexactFloat64())
	fmt.Printf("Max Drawdown:     %.2f%%\n", result.MaxDrawdown.Mul(decimal.NewFromInt(100)).InexactFloat64())
	fmt.Println()
	fmt.Printf("Total Trades:     %d\n", result.TotalTrades)
	fmt.Printf("Winning Trades:   %d\n", result.WinningTrades)
	fmt.Printf("Losing Trades:    %d\n", result.LosingTrades)
	fmt.Printf("Win Rate:         %.2f%%\n", result.WinRate.Mul(decimal.NewFromInt(100)).InexactFloat64())
	fmt.Printf("Profit Factor:    %.2f\n", result.ProfitFactor.InexactFloat64())
}

func printMetrics(m *backtest.Metrics) {
	fmt.Println("\n=== PERFORMANCE METRICS ===")
	fmt.Printf("Sharpe Ratio:     %.2f\n", m.SharpeRatio().InexactFloat64())
	fmt.Printf("Sortino Ratio:    %.2f\n", m.SortinoRatio().InexactFloat64())
	fmt.Printf("Calmar Ratio:     %.2f\n", m.CalmarRatio().InexactFloat64())
	fmt.Printf("Expectancy:       $%.2f\n", m.Expectancy().InexactFloat64())
	fmt.Printf("Avg Win:          $%.2f\n", m.AverageWin().InexactFloat64())
	fmt.Printf("Avg Loss:         $%.2f\n", m.AverageLoss().InexactFloat64())
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "Path to configuration file")
	paperMode := fs.Bool("paper", true, "Paper trading mode (default: true)")
	dataPath := fs.String("data", "", "Path to CSV data file (interactive if empty)")
	strategyName := fs.String("strategy", "", "Strategy (interactive if empty)")
	barDelay := fs.Duration("bar-delay", 100*time.Millisecond, "Delay between bars in simulation")
	interactive := fs.Bool("i", false, "Force interactive mode")
	fs.Parse(args)

	// Interactive mode for strategy
	if *strategyName == "" || *interactive {
		*strategyName = selectStrategy()
	}

	// Interactive mode for data file (paper mode only)
	if *paperMode && (*dataPath == "" || *interactive) {
		*dataPath = selectDataFile()
	}

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mode := "paper"
	if !*paperMode {
		mode = "live"
	}

	slog.Info("quant-bot starting",
		"version", Version,
		"mode", mode,
		"instrument", cfg.Market.InstrumentPrimary,
		"equity", cfg.Account.StartingEquity,
	)

	// Initialize persistence if enabled
	var repo *persistence.SQLiteRepository
	if cfg.Persistence.Enabled {
		repo, err = persistence.NewSQLiteRepository(cfg.Persistence.Path)
		if err != nil {
			slog.Error("failed to initialize persistence", "err", err)
			os.Exit(1)
		}
		defer repo.Close()

		slog.Info("persistence initialized", "path", cfg.Persistence.Path)

		// Attempt recovery from saved state
		if state, err := repo.GetState(ctx); err == nil && state != nil {
			slog.Info("recovered state from persistence",
				"equity", state.Equity,
				"high_water", state.HighWaterMark,
				"kill_switch", state.KillSwitchActive,
				"total_trades", state.TotalTrades,
			)
		}
	}

	// Initialize alerter
	alerter := createAlerter(cfg, logger)

	// Initialize risk engine
	riskEngine := risk.NewEngine(cfg.ToRiskConfig(), cfg.StartingEquityDecimal(), logger)
	_ = riskEngine // TODO: Use in trading loop

	slog.Info("risk engine initialized",
		"max_drawdown", cfg.Account.MaxGlobalDrawdownPct,
		"risk_per_trade", cfg.Account.RiskPerTradePct,
	)

	// Initialize metrics server
	var metricsServer *metrics.Server
	if cfg.Metrics.Enabled {
		metricsCfg := metrics.ServerConfig{
			Port:        cfg.Metrics.Port,
			MetricsPath: cfg.Metrics.Path,
			HealthPath:  "/health",
		}
		metricsServer = metrics.NewServer(metricsCfg, logger)

		// Register health checks
		metricsServer.RegisterHealthCheck("risk_engine", func() metrics.Check {
			if riskEngine.IsInSafeMode() {
				return metrics.Check{Status: "degraded", Message: "safe mode active"}
			}
			return metrics.Check{Status: "healthy"}
		})

		metricsServer.RegisterHealthCheck("persistence", func() metrics.Check {
			if repo == nil {
				return metrics.Check{Status: "healthy", Message: "disabled"}
			}
			return metrics.Check{Status: "healthy"}
		})

		if err := metricsServer.Start(); err != nil {
			slog.Error("failed to start metrics server", "err", err)
		} else {
			slog.Info("metrics server started",
				"port", cfg.Metrics.Port,
				"path", cfg.Metrics.Path,
			)
		}

		// Set build info metric
		metrics.SetBuildInfo(Version, GitCommit, BuildTime)

		// Record initial equity
		metrics.EquityCurrent.Set(cfg.Account.StartingEquity)
		metrics.EquityHighWaterMark.Set(cfg.Account.StartingEquity)
	}

	// Send bot started alert
	if cfg.Alerting.Enabled {
		if err := alerter.Alert(ctx, alerting.SeverityInfo, "Bot started",
			"version", Version,
			"mode", mode,
			"instrument", cfg.Market.InstrumentPrimary,
			"equity", cfg.Account.StartingEquity,
		); err != nil {
			slog.Warn("failed to send start alert", "err", err)
		}
	}

	// Initialize strategy
	var strat strategy.Strategy
	switch *strategyName {
	case "breakout":
		strat = strategy.NewBreakout(strategy.BreakoutConfig{
			LookbackBars:   cfg.Risk.VolatilityLookbackBars,
			ATRMultiplier:  decimal.NewFromFloat(cfg.Risk.StopLossATRMultiple),
			BreakoutBuffer: decimal.Zero,
		})
	case "meanrev":
		strat = strategy.NewMeanReversion(strategy.MeanRevConfig{
			SMAPeriod:     20,
			StdDevPeriod:  20,
			EntryStdDev:   decimal.RequireFromString("2.0"),
			ATRMultiplier: decimal.NewFromFloat(cfg.Risk.StopLossATRMultiple),
		})
	case "grid":
		strat = strategy.NewGrid(strategy.OriginalGridConfig())
	case "grid-conservative":
		strat = strategy.NewGrid(strategy.ConservativeGridConfig())
	default:
		slog.Error("unknown strategy", "name", *strategyName)
		os.Exit(1)
	}

	// Initialize calculator
	calculator := observer.NewCalculator(observer.CalculatorConfig{
		ATRPeriod:    cfg.Risk.VolatilityLookbackBars,
		StdDevPeriod: 20,
	})

	// Initialize broker
	var tradingEngine *engine.Engine
	if *paperMode {
		// Paper trading mode
		paperCfg := paper.Config{
			InitialEquity:     cfg.StartingEquityDecimal(),
			SlippageTicks:     cfg.Backtest.SlippageTicks,
			CommissionPerSide: decimal.NewFromFloat(cfg.Backtest.CommissionPerContract / 2),
			FillDelay:         50 * time.Millisecond,
		}
		paperBroker := paper.NewBroker(paperCfg, logger)

		if err := paperBroker.Connect(ctx); err != nil {
			slog.Error("failed to connect paper broker", "err", err)
			os.Exit(1)
		}

		// Create engine
		engineCfg := engine.Config{
			Symbol:               cfg.Market.InstrumentPrimary,
			Timeframe:            5 * time.Minute,
			EquityUpdateInterval: 1 * time.Minute,
		}
		tradingEngine = engine.NewEngine(
			engineCfg,
			paperBroker,
			riskEngine,
			strat,
			calculator,
			alerter,
			logger,
		)

		// Start engine
		if err := tradingEngine.Start(ctx); err != nil {
			slog.Error("failed to start trading engine", "err", err)
			os.Exit(1)
		}

		// If data file provided, stream it to the paper broker
		if *dataPath != "" {
			go streamDataToPaperBroker(ctx, *dataPath, cfg.Market.InstrumentPrimary, paperBroker, *barDelay, logger)
		} else {
			slog.Warn("no data file provided, paper broker will wait for market data")
		}
	} else {
		slog.Error("live trading not yet implemented")
		os.Exit(1)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutdown signal received")

	// Stop trading engine
	if tradingEngine != nil {
		if err := tradingEngine.Stop(context.Background()); err != nil {
			slog.Error("failed to stop trading engine", "err", err)
		}
	}

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout(),
	)
	defer cancel()

	// Shutdown metrics server
	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("metrics server shutdown error", "err", err)
		}
	}

	// Perform shutdown tasks
	if err := shutdownWithPersistence(shutdownCtx, cfg, repo, riskEngine, alerter); err != nil {
		slog.Error("shutdown error", "err", err)
	}

	// Send shutdown alert
	if cfg.Alerting.Enabled {
		// Use background context since main ctx is cancelled
		alertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := alerter.Alert(alertCtx, alerting.SeverityInfo, "Bot stopped",
			"version", Version,
			"mode", mode,
		); err != nil {
			slog.Warn("failed to send stop alert", "err", err)
		}
		cancel()
	}

	slog.Info("quant-bot shutdown complete")
}

func shutdownWithPersistence(ctx context.Context, cfg *config.Config, repo *persistence.SQLiteRepository, riskEngine *risk.Engine, alerter alerting.Alerter) error {
	_ = alerter // Available for future use in shutdown steps
	slog.Info("starting graceful shutdown",
		"timeout", cfg.ShutdownTimeout(),
	)

	// Shutdown steps with timeout check
	steps := []struct {
		name string
		fn   func() error
	}{
		{"stop trading loop", func() error {
			// TODO: Stop accepting new signals
			return nil
		}},
		{"cancel pending orders", func() error {
			// TODO: Cancel any pending orders
			return nil
		}},
		{"save state", func() error {
			if repo == nil {
				return nil
			}
			// Save current bot state
			state := persistence.BotState{
				LastUpdated:      time.Now(),
				Equity:           riskEngine.CurrentEquity(),
				HighWaterMark:    riskEngine.HighWaterMark(),
				KillSwitchActive: riskEngine.IsInSafeMode(),
				SafeModeActive:   riskEngine.IsInSafeMode(),
			}
			if err := repo.SaveState(ctx, state); err != nil {
				return fmt.Errorf("save bot state: %w", err)
			}
			slog.Info("state saved to persistence",
				"equity", state.Equity,
				"high_water", state.HighWaterMark,
			)
			return nil
		}},
		{"close connections", func() error {
			// TODO: Close broker/data connections
			return nil
		}},
	}

	for _, step := range steps {
		select {
		case <-ctx.Done():
			return fmt.Errorf("shutdown timeout during: %s", step.name)
		default:
			slog.Debug("shutdown step", "step", step.name)
			if err := step.fn(); err != nil {
				slog.Warn("shutdown step failed", "step", step.name, "err", err)
			}
		}
	}

	// Small delay to allow final log messages
	time.Sleep(100 * time.Millisecond)

	return nil
}

// createAlerter creates an alerter based on configuration.
func createAlerter(cfg *config.Config, logger *slog.Logger) alerting.Alerter {
	if !cfg.Alerting.Enabled {
		// Return console alerter for logging only
		return alerting.NewConsoleAlerter(logger)
	}

	var alerters []alerting.Alerter

	// Always add console alerter
	alerters = append(alerters, alerting.NewConsoleAlerter(logger))

	// Add configured channels
	for _, ch := range cfg.Alerting.Channels {
		switch ch.Type {
		case "telegram":
			if ch.BotToken != "" && ch.ChatID != "" {
				telegramAlerter := alerting.NewTelegramAlerter(alerting.TelegramConfig{
					BotToken: ch.BotToken,
					ChatID:   ch.ChatID,
				})
				alerters = append(alerters, telegramAlerter)
				slog.Info("telegram alerter configured", "chat_id", ch.ChatID)
			} else {
				slog.Warn("telegram channel missing bot_token or chat_id")
			}
		default:
			slog.Warn("unknown alert channel type", "type", ch.Type)
		}
	}

	return alerting.NewMultiAlerter(logger, alerters...)
}

// streamDataToPaperBroker streams CSV data to the paper broker for simulation.
func streamDataToPaperBroker(ctx context.Context, dataPath, symbol string, broker *paper.Broker, delay time.Duration, logger *slog.Logger) {
	feed := observer.NewBacktestFeed(dataPath, symbol)

	eventCh, err := feed.Subscribe(ctx, symbol)
	if err != nil {
		logger.Error("failed to subscribe to backtest feed", "err", err)
		return
	}

	logger.Info("starting data stream to paper broker",
		"file", dataPath,
		"symbol", symbol,
		"bar_delay", delay,
	)

	barCount := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("data stream stopped", "bars_sent", barCount)
			return
		case event, ok := <-eventCh:
			if !ok {
				logger.Info("data stream completed", "bars_sent", barCount)
				return
			}

			broker.SimulateMarketData(event)
			barCount++

			if barCount%50 == 0 {
				logger.Info("data stream progress", "bars_sent", barCount)
			}

			// Delay between bars to simulate real-time
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
}
