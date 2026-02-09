package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

func main() {
	var configPath string
	var logLevel string
	flag.StringVar(&configPath, "config", "", "optional path to config JSON file")
	flag.StringVar(&logLevel, "debug-level", "info", "log verbosity: info (default), warn, debug")
	flag.Parse()

	levelText := strings.ToLower(strings.TrimSpace(logLevel))
	switch levelText {
	case "", "info":
		levelText = "info"
	case "warn", "debug":
	default:
		fmt.Fprintf(os.Stderr, "invalid debug-level %q (allowed: info, warn, debug)\n", logLevel)
		os.Exit(2)
	}

	zapConfig := zap.NewProductionConfig()
	if err := zapConfig.Level.UnmarshalText([]byte(levelText)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse debug-level %q: %v\n", logLevel, err)
		os.Exit(2)
	}

	logger, err := zapConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	cfg, err := config.LoadWithPath(logger, configPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	application, err := app.Build(context.Background(), logger, cfg)
	if err != nil {
		logger.Fatal("build app", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Fatal("run app", zap.Error(err))
	}
}
