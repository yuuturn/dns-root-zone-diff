# dns-root-diff

DNS root zone file change notification ツール。

https://www.internic.net/domain/root.zone から DNS root zone ファイルを 6時間に一度取得し、前回との差分を検出して、Slack または X (Twitter) に通知する。

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
- pre-commit: gofmt, go vet, go test, golangci-lint をフックで実行
- CI: GitHub Actions (テスト + lint)

## アーキテクチャ

- `cmd/dns-root-diff/main.go`: エントリーポイント、定期実行ループ
- `internal/fetcher`: HTTP で root zone 取得
- `internal/zone`: zone ファイルパーサー
- `internal/diff`: 新旧レコード差分検出 + カテゴリ分類
- `internal/store`: ローカルディスクへのスナップショット保存
- `internal/notify`: Notifier インターフェース、Slack Webhook、X API v2
- `internal/config`: YAML 設定 + 環境変数オーバーライド
- `deploy/`: systemd unit とデプロイシェル

## 実行方法

```bash
# 単発実行
./bin/dns-root-diff -config config.yaml -once

# 定期実行
./bin/dns-root-diff -config config.yaml
```

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
- VPS では SELinux が有効な場合があるため、バイナリと systemd unit ファイルのラベルを `restorecon` で修正する。
- main への直接 push を防ぐため、GitHub の branch protection rule で "Require a pull request before merging" を有効化する。