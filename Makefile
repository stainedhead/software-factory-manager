.PHONY: build test lint fmt cover check

LDFLAGS := -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

check: fmt lint test
