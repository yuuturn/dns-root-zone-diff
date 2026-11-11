#!/usr/bin/env bash
set -euo pipefail

# デプロイ設定はスクリプトの外で管理する。優先順位は次のとおり:
#   1. 環境変数 (例: VPS=host make deploy)
#   2. deploy/deploy.env (gitignore 済み。deploy/deploy.env.example をコピーして使う)
#   3. このスクリプト内の既定値 (BINARY / REMOTE_BIN / SERVICE のみ)
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${DEPLOY_ENV_FILE:-${SCRIPT_DIR}/deploy.env}"

# 環境変数で明示された値を退避しておき、deploy.env の値より優先する。
_VPS_ENV="${VPS-}"
_BINARY_ENV="${BINARY-}"
_REMOTE_BIN_ENV="${REMOTE_BIN-}"
_SERVICE_ENV="${SERVICE-}"

if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck source=/dev/null
  . "${ENV_FILE}"
fi

VPS="${_VPS_ENV:-${VPS:-}}"
BINARY="${_BINARY_ENV:-${BINARY:-dns-root-diff}}"
REMOTE_BIN="${_REMOTE_BIN_ENV:-${REMOTE_BIN:-/usr/local/bin/${BINARY}}}"
SERVICE="${_SERVICE_ENV:-${SERVICE:-${BINARY}.service}}"

if [[ -z "${VPS}" ]]; then
  echo "error: VPS is not set." >&2
  echo "       Put it in ${ENV_FILE} (see deploy/deploy.env.example)," >&2
  echo "       or pass it via the environment: VPS=<host> $0" >&2
  exit 1
fi

echo "==> Cross-compiling for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o "bin/${BINARY}-linux-amd64" ./cmd/dns-root-diff

echo "==> Uploading to ${VPS}..."
scp "bin/${BINARY}-linux-amd64" "${VPS}:/tmp/${BINARY}"

echo "==> Installing and restarting service..."
ssh "${VPS}" "
  sudo install -o root -g root -m 755 /tmp/${BINARY} ${REMOTE_BIN}
  rm /tmp/${BINARY}
  if command -v restorecon >/dev/null 2>&1; then
    sudo restorecon -v ${REMOTE_BIN} /etc/systemd/system/${SERVICE} 2>/dev/null || true
  fi
  sudo systemctl daemon-reload
  sudo systemctl restart ${BINARY}
  sleep 2
  sudo systemctl status ${BINARY} --no-pager
"

echo "==> Deploy complete."
