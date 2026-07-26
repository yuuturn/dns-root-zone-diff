package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/fetcher"
	"github.com/yfujii/dns-root-diff/internal/notify"
	"github.com/yfujii/dns-root-diff/internal/store"
	"github.com/yfujii/dns-root-diff/internal/web"
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

	webErr := make(chan error, 1)
	if cfg.Web.Enabled {
		srv := &http.Server{
			Handler: web.New(store.NewHistory(cfg.DataDir), web.StaticFS()).Handler(),
		}
		// ポート競合や不正なアドレスを起動時に即検出できるよう bind は同期で行う。
		ln, err := net.Listen("tcp", cfg.Web.Listen)
		if err != nil {
			return fmt.Errorf("web server listen on %s: %w", cfg.Web.Listen, err)
		}
		fmt.Printf("web server listening on %s\n", cfg.Web.Listen)
		go func() {
			if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
				webErr <- err
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				fmt.Fprintf(os.Stderr, "web server shutdown failed: %v\n", err)
			}
		}()
	}

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
		case err := <-webErr:
			// web.enabled な構成で Web UI だけ死んだ縮退状態のまま稼働を続けない。
			// エラー終了して systemd (Restart=on-failure) に再起動させる。
			return fmt.Errorf("web server failed: %w", err)
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
		var tw *notify.TwitterNotifier
		if cfg.Twitter.OAuth2AccessToken != "" {
			persist := func(access, refresh string) error {
				return config.SaveOAuth2Tokens(configPath, access, refresh)
			}
			tw = notify.NewTwitterOAuth2Notifier(
				cfg.Twitter.OAuth2AccessToken,
				cfg.Twitter.OAuth2RefreshToken,
				cfg.Twitter.OAuth2ClientID,
				cfg.Twitter.OAuth2ClientSecret,
				persist,
			)
		} else if cfg.Twitter.APIKey != "" && cfg.Twitter.AccessToken != "" {
			tw = notify.NewTwitterNotifier(cfg.Twitter.APIKey, cfg.Twitter.APISecret, cfg.Twitter.AccessToken, cfg.Twitter.AccessSecret)
		}
		if tw != nil {
			tw.SetMaxPosts(cfg.Twitter.MaxPosts)
			notifiers = append(notifiers, tw)
		}
	}
	if cfg.Bluesky.Enabled && cfg.Bluesky.Handle != "" && cfg.Bluesky.AppPassword != "" {
		notifiers = append(notifiers, notify.NewBlueskyNotifier(cfg.Bluesky))
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
	hadPrevious := s.Exists()

	var oldRecords []zone.Record
	if hadPrevious {
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
	diff.SortChanges(changes)
	if len(changes) == 0 {
		fmt.Println("no changes detected")
	} else {
		fmt.Printf("detected %d changes\n", len(changes))
		// root zone は12時間ごとに再署名され、その際 RRSIG/SOA/ZONEMD が機械的に入れ替わる。
		// 実質的な変更を伴わない回は通知しない (Web UI には履歴として残す)。
		if substantive := diff.Substantive(changes); len(substantive) == 0 {
			fmt.Println("re-signing only; skipping notification")
		} else {
			fmt.Printf("%d substantive changes\n", len(substantive))
			notifiers := buildNotifiers(cfg, configPath)
			for _, n := range notifiers {
				if err := n.Notify(ctx, changes); err != nil {
					fmt.Fprintf(os.Stderr, "notify %s failed: %v\n", n.Name(), err)
				}
			}
		}
		// 初回実行 (前回スナップショットなし) は全レコードが added になるため履歴には残さない。
		if hadPrevious {
			oldSerial, _ := zone.SOASerial(oldRecords)
			newSerial, ok := zone.SOASerial(records)
			if !ok {
				// ID 生成にシリアルが必要なため、SOA が取れない場合のフォールバック。
				newSerial = "unknown"
			}
			h := store.NewHistory(cfg.DataDir)
			entry := store.NewEntry(time.Now().UTC(), oldSerial, newSerial, changes)
			if err := h.Append(entry); err != nil {
				fmt.Fprintf(os.Stderr, "record history failed: %v\n", err)
			}
		}
	}

	if err := s.Save(data); err != nil {
		return fmt.Errorf("save zone: %w", err)
	}

	return nil
}
