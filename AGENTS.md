# dns-root-diff

DNS root zone file change notification ツール。

https://www.internic.net/domain/root.zone から DNS root zone ファイルを 2時間に一度取得し、前回との差分を検出して、Slack または X (Twitter) に通知する。

## コミュニケーション

- 日本語で会話する。

## 実装

- 言語: Go
- バージョン: 1.26.5
- リモートリポジトリ: GitHub (https://github.com/yuuturn/dns-root-zone-diff)
- デプロイ先: VPS (vps1.xsv.yfujii.net, Rocky Linux 10.2, x86_64, systemd 257)
- デプロイ方式: macOS arm64 で GOOS=linux GOARCH=amd64 クロスコンパイル → scp → systemd
- 開発手法: TDD (テスト駆動開発)
- フォーマッター / Linter / 型チェック: golangci-lint v2, go vet, gofmt
- シークレットスキャン: gitleaks v8 (`.gitleaks.toml`)
- pre-commit: gitleaks, gofmt, go vet, go test, golangci-lint をフックで実行
- CI: GitHub Actions (gitleaks + テスト + lint)

## アーキテクチャ

- `cmd/dns-root-diff/main.go`: エントリーポイント、定期実行ループ (zone + root anchors の2系統)
- `internal/fetcher`: HTTP で root zone / root anchors 取得
- `internal/zone`: zone ファイルパーサー
- `internal/anchor`: root anchors (https://data.iana.org/root-anchors/root-anchors.xml) XML パーサー。各 KeyDigest を擬似 DS レコード (Name=キーID, RData=keytag alg digesttype digest [退役日]) に写像し、既存 diff エンジンで差分検出する
- `internal/diff`: 新旧レコード差分検出 + カテゴリ分類
- `internal/store`: ローカルディスクへのスナップショット保存 (root.zone / root-anchors.xml) + diff 履歴の JSON 永続化 (`data_dir/diffs/` = zone、`data_dir/anchor-diffs/` = root anchors)
- `internal/notify`: Notifier インターフェース (Notify=zone / NotifyAnchors=root anchors)、Slack Webhook、X API v2
- `internal/config`: YAML 設定 + 環境変数オーバーライド (`anchor_url` は空なら root anchors 監視無効)
- `internal/web`: diff 履歴閲覧の HTTP API (`/api/diffs` = zone、`/api/anchors/diffs` = root anchors) + 静的配信 (フロントエンドを go:embed)
- `web/frontend/`: Web UI (Vite + React + @cloudflare/kumo)。Root Zone / Root Anchors のタブで履歴を分離表示。ビルド成果物は `internal/web/static/` にコミットする
- `deploy/`: systemd unit、デプロイシェル、nginx リバースプロキシ設定例

## 実行方法

```bash
# 単発実行
./bin/dns-root-diff -config config.yaml -once

# 定期実行
./bin/dns-root-diff -config config.yaml
```

## フロントエンド開発フロー

- `web/frontend/` を変更したら `make frontend-build` を実行し、出力された `internal/web/static/` を**変更と同じコミットに含める**。
- CI はフロントエンドを再ビルドして `git diff --exit-code internal/web/static` で一致を検証するため、ビルド忘れがあると CI が落ちる。
- 通常の Go ビルド・デプロイには Node.js は不要 (成果物コミット済みのため)。
- kumo の standalone CSS には任意の Tailwind ユーティリティが含まれない。レイアウト用クラスは `web/frontend/src/app.css` に定義する。

## デプロイ

```bash
make deploy
```

VPS では事前に `/etc/dns-root-diff/config.yaml` を配置し、`dns-root-diff` ユーザーが読み取める必要がある。

## ブランチ戦略

- `main` への直接 commit は禁止
- 機能ごとに feature ブランチを切る: `git checkout -b feat/xxx`
- PR を作成し、GitHub Actions CI (`ci`) が PASS することを必須とする
- CI 通過後に main へ merge する
- main ブランチは branch protection rule で直接 push を防止

```bash
# 作業例
git checkout -b feat/update-notifier
# ... 変更 ...
git commit -m "feat: update notifier"
git push -u origin feat/update-notifier
# GitHub で PR 作成、CI 通過後に merge
```

## 注意事項

- 設定ファイルは秘密情報を含む可能性があるため、パーミッションを適切に管理する。
- トークンや Webhook URL をリポジトリに commit しない。`config.yaml` は gitignore 済み。
- コミット前に gitleaks が秘密情報を検出する。ローカル確認は `make secrets`。
- VPS では SELinux が有効な場合があるため、バイナリと systemd unit ファイルのラベルを `restorecon` で修正する。
- Web UI を nginx で公開する場合、SELinux では `setsebool -P httpd_can_network_connect 1` が必要 (`deploy/nginx.conf.example` 参照)。
- main への直接 push を防ぐため、GitHub の branch protection rule で "Require a pull request before merging" を有効化する。