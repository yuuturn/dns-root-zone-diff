package main

import (
	"flag"
	"fmt"
	"os"

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
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("config loaded: zone_url=%s, interval=%v, data_dir=%s\n", cfg.ZoneURL, cfg.FetchInterval, cfg.DataDir)
	return nil
}
