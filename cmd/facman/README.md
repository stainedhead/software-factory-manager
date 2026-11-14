# cmd/facman

Entry point for the `facman` binary (the Agentic Software Factory Manager). Keep this package thin: wire dependencies from `internal/` and run. No business logic; commands and flags live in [`internal/adapter/cli`](../../internal/adapter/cli/).

| File | Purpose |
|---|---|
| `main.go` | `main` calls `run(args, defaultOptions())`, which builds the app and returns its exit code |
| `wire.go` | `options` (stdio, environment, OS facts, UID, known prefixes, installer URL, HTTP client, clock, user, signals, TUI) and `defaultOptions()` with the real values; `newApp(options)` builds the runner, backends, detectors, bootstrap service, state, catalog, installer and the session, and returns a `cli.App` |
| `main_test.go` | `--version`, `--about`, unknown flags |
| `harness_test.go` | e2e harness: a fake machine (OS, WSL, display, glibc, brew on or off PATH or missing, UID) built from `internal/testkit` fakecmd, an `httptest` installer, a fixed clock and user; facts go in through `options`, never test-only flags or env vars |
| `e2e_test.go` | In-process e2e tests: scan JSON against schema v1, exit codes, plan and dry run, apply with consent and audit, decline, conflicts, doctor, no Homebrew, Linuxbrew, headless Linux and WSL, a second mutating run (4), signals, installer failure |
| `crash_test.go` | Crash recovery: the test binary re-runs itself as a facman child and is killed mid-command |

Depends on `internal/adapter/...` and `internal/domain/appinfo`; nothing depends on this package. Build with `make build`. Back to the [root README](../../README.md).
