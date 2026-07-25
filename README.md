# DNS Root Zone Diff

DNS root zone の変更を機械的に検知して通知するツール。

## 機能

- https://www.internic.net/domain/root.zone から 2時間に一度ゾーン取得
- 前回との差分を検出
- 変更をカテゴリ別に整理（delegation / DNSSEC / other）
- Slack Webhook へ通知
- X (Twitter) API v2 へ通知
- diff 履歴を Web UI で閲覧 (Cloudflare [kumo](https://github.com/cloudflare/kumo) 製)

## ローカルインストール

```bash
brew install go golangci-lint pre-commit gh
pre-commit install
```

## 設定

`config.yaml` を作成:

```yaml
zone_url: "https://www.internic.net/domain/root.zone"
fetch_interval: "2h"
data_dir: "/var/lib/dns-root-diff"
slack:
  enabled: false
  webhook_url: ""
twitter:
  enabled: false
  api_key: ""
  api_secret: ""
  access_token: ""
  access_secret: ""
web:
  enabled: false
  listen: "127.0.0.1:8080"
```

環境変数で上書き可能:

- `DNS_ROOT_DIFF_ZONE_URL`
- `DNS_ROOT_DIFF_INTERVAL`
- `DNS_ROOT_DIFF_DATA_DIR`
- `SLACK_WEBHOOK_URL`
- `TWITTER_API_KEY`
- `TWITTER_API_SECRET`
- `TWITTER_ACCESS_TOKEN`
- `TWITTER_ACCESS_SECRET`
- `DNS_ROOT_DIFF_WEB_ENABLED`
- `DNS_ROOT_DIFF_WEB_LISTEN`

## 実行

```bash
# 単発実行
make build
./bin/dns-root-diff -config config.yaml -once

# 定期実行
./bin/dns-root-diff -config config.yaml
```

## Web UI

`web.enabled: true` にすると定期実行モード時に diff 履歴を閲覧できる Web サーバーが起動する
(`-once` では起動しない)。履歴は変更検知のたびに `data_dir/diffs/` へ JSON として保存される。

- `GET /` : 一覧・詳細画面 (React + [@cloudflare/kumo](https://github.com/cloudflare/kumo))
- `GET /api/diffs?page=1&per_page=20` : diff 履歴一覧
- `GET /api/diffs/{id}` : diff 詳細
- `GET /api/health` : 死活監視

フロントエンドのビルド成果物は `internal/web/static/` にコミットされ、go:embed で
バイナリに埋め込まれるため、通常のビルド・デプロイに Node.js は不要。
フロントエンド (`web/frontend/`) を変更した場合は再ビルドして成果物ごとコミットする:

```bash
make frontend-install  # 初回のみ
make frontend-build    # internal/web/static に出力される
```

インターネットへ公開する場合は nginx で TLS 終端するリバースプロキシを推奨。
設定例と SELinux / firewalld の手順は [deploy/nginx.conf.example](deploy/nginx.conf.example) を参照。

## テスト

```bash
make test
make lint
```

## VPS へのデプロイ

```bash
make deploy
```

VPS には事前に専用ユーザーとデータディレクトリを作成して config.yaml を配置する必要があります。
