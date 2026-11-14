# .github

Repository automation and agent instructions.

| Path | Purpose |
|---|---|
| `workflows/ci.yml` | On every PR and push to `main`: tidy check, lint, tests with coverage, integration tests, build and smoke test on Linux and macOS |
| `workflows/release.yml` | On a `v*` tag: runs `make check`, then GoReleaser publishes binaries and checksums to GitHub Releases |
| `copilot-instructions.md` | Points Copilot at [../AGENTS.md](../AGENTS.md) |

GoReleaser config is [../.goreleaser.yaml](../.goreleaser.yaml). Release process and CI rules are in [../AGENTS.md](../AGENTS.md). Signing, notarization and the Homebrew tap are not set up yet (see the [design](../docs/design/software-factory-installer-design.md)). Back to the [root README](../README.md).
