# cmd/facman

Entry point for the `facman` binary (the Agentic Software Factory Manager). Keep this package thin: parse flags, wire dependencies from `internal/`, and run. No business logic.

| File | Purpose |
|---|---|
| `main.go` | Program entry point; handles `--version` and `--about` (also the no-argument output) using [`internal/domain/appinfo`](../../internal/domain/appinfo/) |
| `main_test.go` | Tests for the entry point |

Depends on `internal/domain/appinfo`; nothing depends on this package. Build with `make build`. Back to the [root README](../../README.md).
