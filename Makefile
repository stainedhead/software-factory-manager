.PHONY: build test lint fmt cover check

build:
	go build -o bin/ ./cmd/...

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
