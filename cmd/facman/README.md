# cmd/facman

Entry point for the `facman` binary (the Agentic Software Factory Manager). Keep this package thin: wire dependencies from `internal/` and run. No business logic; commands and flags live in [`internal/adapter/cli`](../../internal/adapter/cli/).

| File | Purpose |
|---|---|
| `main.go` | `main` calls `run(args, defaultOptions())`, which builds the app and returns its exit code |
| `wire.go` | `options` (stdio, environment, OS facts, UID, known prefixes, installer URL, HTTP client, clock, user, signals, TUI) and `defaultOptions()` with the real values; `newApp(options)` builds the runner, backends, detectors, bootstrap service, state, catalog, installer and the session, and the TUI (`adapter/tui` over the session, with the runner's `Handoff` for sudo commands), and returns a `cli.App` |
| `main_test.go` | `--version`, `--about`, unknown flags |
| `harness_test.go` | e2e harness: a fake machine (OS, WSL, display, glibc, brew on or off PATH or missing, UID) built from `internal/testkit` fakecmd, an `httptest` installer, a fixed clock and user; facts go in through `options`, never test-only flags or env vars |
| `e2e_test.go` | In-process e2e tests: scan JSON against schema v1, exit codes, plan and dry run, apply with consent and audit, decline, conflicts, doctor, no Homebrew, Linuxbrew, headless Linux and WSL, a second mutating run (4), signals, installer failure, and the TUI on a pseudo-terminal (full install through the fake brew; `q` changes nothing; stdin not a terminal is 64) |
| `integration_test.go` | `integration` build tag (`make integration`, spec I1-14): `scan`, `plan` and `doctor` against the machine's real Homebrew with a temp `HOME`, so the state directory is temporary (skips without brew or as root). With `FACMAN_INTEGRATION_MUTATE=1` on macOS only: `apply --profile integration --catalog testdata/catalogs/integration` installs GNU `hello` through a pty `y`, asserts a `managed` record, and uninstalls it. It skips if `hello` is already installed or the prefix belongs to another user. `TestIntegrationCatalogLoads` in `e2e_test.go` validates that catalog on every run. |
| `perf_test.go` | Performance smoke: `--version` in process and as a built binary |
| `crash_test.go` | Crash recovery: the test binary re-runs itself as a facman child and is killed mid-command |

Depends on `internal/adapter/...` and `internal/domain/appinfo`; nothing depends on this package. Build with `make build`, or `make build-all` for every release platform. Back to the [root README](../../README.md).
