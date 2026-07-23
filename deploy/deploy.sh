#!/usr/bin/env bash
set -euo pipefail

VPS="vps1.xsv.yfujii.net"
BINARY="dns-root-diff"
REMOTE_BIN="/usr/local/bin/${BINARY}"
SERVICE="${BINARY}.service"

echo "==> Cross-compiling for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o "bin/${BINARY}-linux-amd64" ./cmd/dns-root-diff

echo "==> Uploading to ${VPS}..."
scp "bin/${BINARY}-linux-amd64" "${VPS}:/tmp/${BINARY}"

echo "==> Installing and restarting service..."
ssh "${VPS}" "
  sudo install -o root -g root -m 755 /tmp/${BINARY} ${REMOTE_BIN}
  rm /tmp/${BINARY}
  sudo systemctl daemon-reload
  sudo systemctl restart ${BINARY}
  sleep 2
  sudo systemctl status ${BINARY} --no-pager
"

echo "==> Deploy complete."
