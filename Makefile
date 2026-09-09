BINARY := tfc-vault
PREFIX ?= $(HOME)/.local

.PHONY: build test vet fmt install clean

build:
	go build -ldflags "-X github.com/iwashitahga/tfc-vault/internal/cli.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)" -o dist/$(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

install: build
	install -d $(PREFIX)/bin
	install -m 0755 dist/$(BINARY) $(PREFIX)/bin/$(BINARY)

clean:
	rm -rf dist
