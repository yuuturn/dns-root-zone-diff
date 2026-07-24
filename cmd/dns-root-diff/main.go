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
	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/fetcher"
	"github.com/yfujii/dns-root-diff/internal/notify"
	"github.com/yfujii/dns-root-diff/internal/store"
	"github.com/yfujii/dns-root-diff/internal/zone"
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
		return runOnce(context.Background(), cfg, *configPath)
	}

	return runLoop(cfg, *configPath)
}

func runLoop(cfg config.Config, configPath string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(cfg.FetchInterval)
	defer ticker.Stop()

	if err := runOnce(ctx, cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "initial run failed: %v\n", err)
	}

	for {
		select {
		case <-ticker.C:
			// Reload config each tick so refreshed OAuth tokens are picked up.
			if configPath != "" {
				if reloaded, err := config.Load(configPath); err == nil {
					cfg = reloaded
				}
			}
			if err := runOnce(ctx, cfg, configPath); err != nil {
				fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
			}
		case <-ctx.Done():
			fmt.Println("shutting down")
			return nil
		}
	}
}

func buildNotifiers(cfg config.Config, configPath string) []notify.Notifier {
	var notifiers []notify.Notifier
	if cfg.Slack.Enabled && cfg.Slack.WebhookURL != "" {
		notifiers = append(notifiers, notify.NewSlackNotifier(cfg.Slack.WebhookURL))
	}
	if cfg.Twitter.Enabled {
		if cfg.Twitter.OAuth2AccessToken != "" {
			persist := func(access, refresh string) error {
				return config.SaveOAuth2Tokens(configPath, access, refresh)
			}
			notifiers = append(notifiers, notify.NewTwitterOAuth2Notifier(
				cfg.Twitter.OAuth2AccessToken,
				cfg.Twitter.OAuth2RefreshToken,
				cfg.Twitter.OAuth2ClientID,
				cfg.Twitter.OAuth2ClientSecret,
				persist,
			))
		} else if cfg.Twitter.APIKey != "" && cfg.Twitter.AccessToken != "" {
			notifiers = append(notifiers, notify.NewTwitterNotifier(cfg.Twitter.APIKey, cfg.Twitter.APISecret, cfg.Twitter.AccessToken, cfg.Twitter.AccessSecret))
		}
	}
	return notifiers
}

func runOnce(ctx context.Context, cfg config.Config, configPath string) error {
	fmt.Printf("fetching zone from %s\n", cfg.ZoneURL)

	f := fetcher.New(cfg.ZoneURL, 2*time.Minute)
	data, err := f.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("fetch zone: %w", err)
	}

	records, err := zone.Parse(data)
	if err != nil {
		return fmt.Errorf("parse zone: %w", err)
	}
	fmt.Printf("parsed %d records\n", len(records))

	s := store.New(cfg.DataDir)

	var oldRecords []zone.Record
	if s.Exists() {
		oldData, err := s.Load()
		if err != nil {
			return fmt.Errorf("load previous zone: %w", err)
		}
		oldRecords, err = zone.Parse(oldData)
		if err != nil {
			return fmt.Errorf("parse previous zone: %w", err)
		}
	}

	changes := diff.Diff(oldRecords, records)
	if len(changes) == 0 {
		fmt.Println("no changes detected")
	} else {
		fmt.Printf("detected %d changes\n", len(changes))
		notifiers := buildNotifiers(cfg, configPath)
		for _, n := range notifiers {
			if err := n.Notify(ctx, changes); err != nil {
				fmt.Fprintf(os.Stderr, "notify %s failed: %v\n", n.Name(), err)
			}
		}
	}

	if err := s.Save(data); err != nil {
		return fmt.Errorf("save zone: %w", err)
	}

	return nil
}
