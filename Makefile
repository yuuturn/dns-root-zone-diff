.PHONY: build build-linux test lint fmt vet secrets ci clean deploy frontend-install frontend-build frontend-check

BINARY       := dns-root-diff
VPS          := vps1.osk.skr.yfujii.net
FRONTEND_DIR := web/frontend

build:
	go build -o bin/$(BINARY) ./cmd/dns-root-diff

build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 ./cmd/dns-root-diff

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w -s .

vet:
	go vet ./...

secrets:
	gitleaks detect --source . --verbose

ci: secrets vet lint test build

clean:
	rm -rf bin/

# フロントエンド (成果物は internal/web/static に出力され go:embed で埋め込まれる。
# フロントエンド変更時は frontend-build して internal/web/static を含めてコミットする)
frontend-install:
	cd $(FRONTEND_DIR) && npm ci

frontend-build:
	cd $(FRONTEND_DIR) && npm run build

frontend-check:
	cd $(FRONTEND_DIR) && npm run typecheck

deploy: build-linux
	scp bin/$(BINARY)-linux-amd64 $(VPS):/tmp/$(BINARY)
	ssh $(VPS) "sudo install -o root -g root -m 755 /tmp/$(BINARY) /usr/local/bin/$(BINARY) && rm /tmp/$(BINARY) && sudo systemctl restart $(BINARY)"
