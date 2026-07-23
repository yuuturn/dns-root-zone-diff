# dns-root-diff

DNS root zone の変更を機械的に検知して通知するツール。

## 機能

- https://www.internic.net/domain/root.zone から 6時間に一度ゾーン取得
- 前回との差分を検出
- 変更をカテゴリ別に整理（delegation / DNSSEC / other）
- Slack Webhook へ通知
- X (Twitter) API v2 へ通知

## ローカルインストール

```bash
brew install go golangci-lint pre-commit gh
pre-commit install
```

## 設定

`config.yaml` を作成:

```yaml
zone_url: "https://www.internic.net/domain/root.zone"
fetch_interval: "6h"
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

## 実行

```bash
# 単発実行
make build
./bin/dns-root-diff -config config.yaml -once

# 定期実行
./bin/dns-root-diff -config config.yaml
```

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
