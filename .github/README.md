# .github

Repository automation and agent instructions.

| Path | Purpose |
|---|---|
| `workflows/ci.yml` | On every PR and push to `main`, on Linux and macOS: `go mod tidy` check, lint (`golangci-lint`, which also lints the `integration`-tagged files), `make cover`, `make integration`, `make archcheck`, `make build` and a `bin/facman --version` smoke test; on Linux also `make fuzz` (10 s per parser) and `make build-all` with a check that the four release binaries exist |
| `workflows/release.yml` | On a `v*` tag: runs `make check`, then GoReleaser publishes binaries and checksums to GitHub Releases |
| `copilot-instructions.md` | Points Copilot at [../AGENTS.md](../AGENTS.md) |

### Integration tests

`make integration` runs `go test -race -tags integration -v -run '^TestIntegration' ./...`, verbose so the log shows `--- PASS` or `--- SKIP` for each `TestIntegration*` (unit tests run in `make cover`). The real-Homebrew tests in `cmd/facman/integration_test.go` and the `brew info` contract test in `internal/adapter/brew/contract_integration_test.go` (spec I1-14) skip when Homebrew is missing or when running as root, but **fail** in those cases when `FACMAN_INTEGRATION_MUTATE=1`, so the macOS job cannot pass by skipping; otherwise they run `scan`, `plan` and `doctor` read-only with a temporary state directory. The one test that changes the machine installs GNU `hello` through `--catalog testdata/catalogs/integration` and uninstalls it afterwards. It runs only when `FACMAN_INTEGRATION_MUTATE=1`, which `ci.yml` sets on macOS runners only, and it skips when `hello` is already installed or the Homebrew prefix belongs to another user. Never set the variable on a machine you care about.

GoReleaser config is [../.goreleaser.yaml](../.goreleaser.yaml). Release process and CI rules are in [../AGENTS.md](../AGENTS.md). Signing, notarization and the Homebrew tap are not set up yet (see the [design](../docs/design/software-factory-installer-design.md)). Back to the [root README](../README.md).
