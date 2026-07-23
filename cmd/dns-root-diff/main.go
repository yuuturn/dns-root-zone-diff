package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to config file")
	once := flag.Bool("once", false, "run once and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if *once {
		return runOnce(context.Background(), cfg)
	}

	return runLoop(cfg)
}

func runLoop(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(cfg.FetchInterval)
	defer ticker.Stop()

	if err := runOnce(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "initial run failed: %v\n", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := runOnce(ctx, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			}
		case <-ctx.Done():
			fmt.Println("shutting down")
			return nil
		}
	}
}

func runOnce(ctx context.Context, cfg config.Config) error {
	fmt.Printf("fetching zone from %s\n", cfg.ZoneURL)
	return nil
}
