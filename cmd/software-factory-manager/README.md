# cmd/software-factory-manager

Entry point for the `software-factory-manager` binary. Keep this package thin: parse flags, wire dependencies from `internal/`, and run. No business logic.

| File | Purpose |
|---|---|
| `main.go` | Program entry point; currently a stub that prints the version |
| `main_test.go` | Tests for the entry point |

Depends on `internal/` (not yet created); nothing depends on this package. Build with `make build`. Back to the [root README](../../README.md).
