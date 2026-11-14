# Technical Details

Increment 1 is being built (see [specs/261114-software-factory-manager/](../specs/261114-software-factory-manager/)); the full component description lands with T-H3. The design's architecture maps onto the Clean Architecture layers in [AGENTS.md](../AGENTS.md) as follows.

| Design component ([§4](design/software-factory-installer-design.md#4-architecture)) | Layer |
|---|---|
| Package, ownership state, plan, action, consent | `internal/domain` |
| Planner/reconciler, manifest loading rules, apply and setup flows | `internal/usecase` (declares Backend, Detector, Inventory and Runner ports) |
| Homebrew formula/cask, npm, script and dmg/pkg backends; detectors; SQLite inventory (`modernc.org/sqlite`); service manager (launchd/systemd) | `internal/adapter` |
| CLI (cobra: commands, flag normalizer, `[y/N]` prompt, rendering, exit codes), report schema v1, the session that composes the use cases and holds the state lock; TUI (Bubble Tea, workstream G) | `internal/adapter/cli`, `internal/adapter/report`, `internal/adapter/session` |
| Wiring only: `options` and `newApp` build every adapter | `cmd/facman` (`main.go`, `wire.go`) |

**Run flow (headless).** `cli.App` validates flags (usage 64 before any I/O), then `session.Run`: bootstrap check (4), state directory and lock for mutating runs (4), catalog and profiles (3), scan, then plan → consent → execute, or status, or doctor. `--json` writes a schema v1 report to stdout. Exit-code precedence 64 > 4 > 3 > 1.

Manifest format: [§5](design/software-factory-installer-design.md#5-manifest-format). State store: [§6](design/software-factory-installer-design.md#6-state-store-inventory). Language choice: [§8](design/software-factory-installer-design.md#8-language-and-framework-recommendation). This page will describe the real components, data flow and dependencies as they are built.
