.PHONY: build build-web build-go build-linux clean lint help

## build: build everything locally (web + go)
build: build-web build-go

## build-web: build React admin UI
build-web:
	cd web && npm ci && npm run build

## build-go: compile binaries for current OS
build-go:
	mkdir -p bin
	go build -ldflags="-s -w" -o bin/subforge       ./cmd/subforge
	go build -ldflags="-s -w" -o bin/subforge-agent ./cmd/agent

## build-linux: cross-compile for Linux amd64 + arm64 (for releases)
build-linux: build-web
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/subforge-linux-amd64       ./cmd/subforge
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/subforge-agent-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/subforge-linux-arm64       ./cmd/subforge
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/subforge-agent-linux-arm64 ./cmd/agent
	@echo "Built:"; ls -lh bin/

## lint: run go vet
lint:
	go vet ./...

## clean: remove build artifacts
clean:
	rm -rf bin/ web/dist/assets/

## help: show this message
help:
	@grep -E '^## ' Makefile | sed 's/## //'
