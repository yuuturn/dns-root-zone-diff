# DNS Root Zone Diff

DNS root zone の変更を機械的に検知して通知するツール。

## 機能

- https://www.internic.net/domain/root.zone から 2時間に一度ゾーン取得
- 前回との差分を検出
- 変更をカテゴリ別に整理（delegation / DNSSEC / glue / signature / zone / other）
- 再署名ノイズを除いた実質的な変更をサマリーにして Slack Webhook へ通知
- 同じサマリーを X (Twitter) API v2 へ 280文字ごとに分割して連投
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
  # 1回の検知で連投する最大ツイート数 (省略時 4)
  max_posts: 4
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
- `TWITTER_MAX_POSTS`
- `DNS_ROOT_DIFF_WEB_ENABLED`
- `DNS_ROOT_DIFF_WEB_LISTEN`

## 変更のカテゴリと通知の内容

検知した変更は RR type でカテゴリ分けする。

| カテゴリ | 対象 RR type |
| --- | --- |
| `delegation` | NS |
| `DNSSEC` | DS, DNSKEY, NSEC, NSEC3PARAM |
| `glue` | A, AAAA |
| `signature` | RRSIG |
| `zone` | SOA, ZONEMD |
| `other` | 上記以外 |

root zone は12時間ごとに再署名され、その際 RRSIG 約2,800レコードが入れ替わって
SOA serial と ZONEMD の serial/digest も更新される。この**機械的な変更**
(差分5,000件超になる) だけの回は **Slack / X へ通知しない**。Web UI には全件が
履歴として残る。

機械的な変更と判定するのは次の3つだけで、同じ RR type でもそれ以外の変更は通知する。

- RRSIG の入れ替え
- SOA の serial だけが変わった modified (MNAME/RNAME/refresh/retry/expire/minimum
  や TTL が変わった場合、追加・削除の場合は通知する)
- ZONEMD の serial と digest だけが変わった modified (scheme/hash algorithm や TTL が
  変わった場合、追加・削除の場合は通知する)

実質的な変更があった回は次の形式で通知する。X は280文字ごと、Slack は3,500文字ごとに
分割し、2通以上になる場合はタイトル行に `(1/3)` のような番号を付ける。

```
DNS Root Zone changes (1/2)
serial 2026072501 -> 2026072502
delegation 2 / DNSSEC 1 / glue 2
re-signing: 2800 RRSIG (omitted)

[delegation]
  - gone. NS ns1.dns.nic.gone.
  + newgtld. NS ns1.dns.nic.newgtld.
[DNSSEC]
  + newgtld. DS 12345 8 2 A1B2C3D4E5F60718293A4B5C6D7E8F...
```

明細はレコード単位で載せる。上限 (`max_posts`) に収まらない場合は TLD ごとの集約
(`  example. NS +1 -1`) に切り替え、それでも収まらない場合は末尾に
`... +N more changes` として落とした件数を明記する。

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
