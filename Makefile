.PHONY: build build-linux test lint fmt vet ci clean deploy

BINARY := dns-root-diff
VPS    := vps1.xsv.yfujii.net

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

ci: vet lint test build

clean:
	rm -rf bin/

deploy: build-linux
	scp bin/$(BINARY)-linux-amd64 $(VPS):/tmp/$(BINARY)
	ssh $(VPS) "sudo install -o root -g root -m 755 /tmp/$(BINARY) /usr/local/bin/$(BINARY) && rm /tmp/$(BINARY) && sudo systemctl restart $(BINARY)"
