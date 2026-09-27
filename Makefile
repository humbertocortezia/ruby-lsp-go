SHELL := /bin/bash
GO ?= go
BIN := vscode-extension/bin/ruby-lsp-go

.PHONY: all build test vet fmt clean run package

all: vet test build

build:
	CGO_ENABLED=0 $(GO) build -ldflags="-s -w" -o $(BIN) .
	chmod +x $(BIN)

vet:
	CGO_ENABLED=0 $(GO) vet ./...

test:
	CGO_ENABLED=0 $(GO) test ./...

fmt:
	$(GO) fmt ./...

run: build
	$(BIN)

package: build
	./vscode-extension/build.sh

clean:
	rm -f $(BIN) vscode-extension/vscode-ruby-lsp-go.vsix
