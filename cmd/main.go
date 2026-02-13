package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

type cliOptions struct {
	ConfigPath string
	LogLevel   string
	Agent      string
}

var buildWithOptions = app.BuildWithOptions

func main() {
	opts, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}

	levelText := opts.LogLevel
	zapConfig := zap.NewProductionConfig()
	if err := zapConfig.Level.UnmarshalText([]byte(levelText)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse debug-level %q: %v\n", opts.LogLevel, err)
		os.Exit(2)
	}

	logger, err := zapConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	cfg, err := config.LoadWithPath(logger, opts.ConfigPath)
	if err != nil {
		logger.Fatal("load config", zap.Error(err))
	}

	application, err := buildApplication(context.Background(), logger, cfg, opts.Agent)
	if err != nil {
		logger.Fatal("build app", zap.Error(err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		logger.Fatal("run app", zap.Error(err))
	}
}

func parseCLIOptions(args []string) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("yaralpho", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.ConfigPath, "config", "", "optional path to config JSON file")
	fs.StringVar(&opts.LogLevel, "debug-level", "info", "log verbosity: info (default), warn, debug")
	fs.StringVar(&opts.Agent, "agent", "codex", "agent provider: codex (default), github")
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	levelText := strings.ToLower(strings.TrimSpace(opts.LogLevel))
	switch levelText {
	case "", "info":
		opts.LogLevel = "info"
	case "warn", "debug":
		opts.LogLevel = levelText
	default:
		return cliOptions{}, fmt.Errorf("invalid debug-level %q (allowed: info, warn, debug)", opts.LogLevel)
	}

	agent := strings.ToLower(strings.TrimSpace(opts.Agent))
	switch agent {
	case "", "codex":
		opts.Agent = "codex"
	case "github":
		opts.Agent = "github"
	default:
		return cliOptions{}, fmt.Errorf("invalid agent %q (allowed: codex, github)", opts.Agent)
	}

	return opts, nil
}

func buildApplication(ctx context.Context, logger *zap.Logger, cfg config.Config, agent string) (*app.App, error) {
	return buildWithOptions(ctx, logger, cfg, app.BuildOptions{Agent: agent})
}
