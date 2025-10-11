package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/app"
	"github.com/kapu/hololive-kakao-bot-go/internal/config"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := util.NewLogger(cfg.Logging.Level, cfg.Logging.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Hololive KakaoTalk Bot starting...",
		zap.String("version", "1.0.0-go"),
		zap.String("log_level", cfg.Logging.Level),
	)

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 30*time.Second)
	container, err := app.Build(buildCtx, cfg, logger)
	buildCancel()
	if err != nil {
		logger.Error("Failed to assemble application services", zap.Error(err))
		os.Exit(1)
	}

	kakaoBot, err := container.NewBot()
	if err != nil {
		logger.Error("Failed to initialize bot", zap.Error(err))
		os.Exit(1)
	}

	// Create context with cancellation for runtime lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start bot in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := kakaoBot.Start(ctx); err != nil {
			errCh <- err
		}
	}()

	logger.Info("Bot started, waiting for signals...")

	// Wait for termination signal or error
	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errCh:
		logger.Error("Bot error", zap.Error(err))
	}

	// Graceful shutdown
	logger.Info("Shutting down gracefully...")
	cancel()

	// Give bot time to cleanup
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := kakaoBot.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
	}

	logger.Info("Shutdown complete")
}
