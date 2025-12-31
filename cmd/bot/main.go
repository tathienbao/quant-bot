// Package main is the entry point for the quant trading bot.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tathienbao/quant-bot/internal/backtest"
	"github.com/tathienbao/quant-bot/internal/config"
	"github.com/tathienbao/quant-bot/internal/execution"
	"github.com/tathienbao/quant-bot/internal/observer"
	"github.com/tathienbao/quant-bot/internal/risk"
	"github.com/tathienbao/quant-bot/internal/strategy"
)

// Version information (set by build flags).
var (
	Version   = "0.4.0"
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
	dataPath := fs.String("data", "", "Path to CSV data file (required)")
	strategyName := fs.String("strategy", "breakout", "Strategy: breakout, meanrev")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	if *dataPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --data is required")
		fs.Usage()
		os.Exit(1)
	}

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

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
	default:
		slog.Error("unknown strategy", "name", *strategyName)
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

	slog.Info("starting backtest",
		"data", *dataPath,
		"strategy", *strategyName,
		"equity", cfg.Account.StartingEquity,
	)

	// Run backtest
	ctx := context.Background()
	result, err := runner.Run(ctx)
	if err != nil {
		slog.Error("backtest failed", "err", err)
		os.Exit(1)
	}

	// Print results
	printBacktestResults(result, cfg.Account.StartingEquity)

	// Calculate metrics
	metrics := backtest.NewMetrics(result, decimal.Zero)
	printMetrics(metrics)
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
	fs.Parse(args)

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

	// Initialize risk engine
	riskEngine := risk.NewEngine(cfg.ToRiskConfig(), cfg.StartingEquityDecimal(), logger)
	_ = riskEngine // TODO: Use in trading loop

	slog.Info("risk engine initialized",
		"max_drawdown", cfg.Account.MaxGlobalDrawdownPct,
		"risk_per_trade", cfg.Account.RiskPerTradePct,
	)

	// TODO: Phase 5+ implementations
	// - Initialize broker connection
	// - Start market data feed
	// - Start trading loop

	slog.Warn("live trading not yet implemented, waiting for shutdown signal...")

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutdown signal received")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout(),
	)
	defer cancel()

	// Perform shutdown tasks
	if err := shutdown(shutdownCtx, cfg); err != nil {
		slog.Error("shutdown error", "err", err)
	}

	slog.Info("quant-bot shutdown complete")
}

func shutdown(ctx context.Context, cfg *config.Config) error {
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
			// TODO: Persist current state
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
