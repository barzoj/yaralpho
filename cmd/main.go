package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "optional path to config JSON file")
	flag.Parse()

	logger, err := zap.NewProduction()
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
