.PHONY: build build-all test integration lint fmt cover check

LDFLAGS := -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

# Static binaries only: facman ships as one self-contained executable (FR-003).
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...

# Cross-compile the GoReleaser matrix into bin/<os>-<arch>/facman.
build-all:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		echo "building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o bin/$$os-$$arch/facman ./cmd/facman || exit 1; \
	done

test:
	go test -race ./...

# Integration tests (TestIntegration*), verbose so the log shows PASS or SKIP for each.
# They skip without Homebrew or as root; FACMAN_INTEGRATION_MUTATE=1 runs the install
# test and turns those skips into failures. Unit tests run in make test / make cover.
integration:
	go test -race -tags integration -v -run '^TestIntegration' ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

check: fmt lint test
