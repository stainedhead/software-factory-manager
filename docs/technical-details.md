# Technical Details

No code exists yet. The planned architecture is in the design doc, and it maps onto the Clean Architecture layers in [AGENTS.md](../AGENTS.md) as follows.

| Design component ([§4](design/software-factory-installer-design.md#4-architecture)) | Layer |
|---|---|
| Package, ownership state, plan, action, consent | `internal/domain` |
| Planner/reconciler, manifest loading rules, apply and setup flows | `internal/usecase` (declares Backend, Detector, Inventory and Runner ports) |
| Homebrew formula/cask, npm, script and dmg/pkg backends; detectors; SQLite inventory (`modernc.org/sqlite`); service manager (launchd/systemd) | `internal/adapter` |
| TUI (Bubble Tea) and CLI (cobra) wiring | `cmd/facman` |

Manifest format: [§5](design/software-factory-installer-design.md#5-manifest-format). State store: [§6](design/software-factory-installer-design.md#6-state-store-inventory). Language choice: [§8](design/software-factory-installer-design.md#8-language-and-framework-recommendation). This page will describe the real components, data flow and dependencies as they are built.
