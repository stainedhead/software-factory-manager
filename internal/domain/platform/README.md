# internal/domain/platform

Platform facts (`Platform`, `Key`, `Distro`) and the startup bootstrap result (`BootstrapReport`, `Finding`, `HomebrewInfo`, `NodeInfo`). `BootstrapReport.Fatal(cmd)` returns the findings that stop a command: `SeverityFatal` findings (such as `uid0`) stop every command, and findings with `BlocksApply` (`linux-prereqs`, `brew-prefix-owner`) stop only `apply` and the TUI Run step.

| File | Contents |
|---|---|
| `platform.go` | `Platform`, `Key()`, `GlibcBelow`, `Command`, `Severity`, finding codes, `Finding`, `BootstrapReport` |

**Architecture.** Domain layer: no I/O. Imports `version`. Used by `catalog` (install selection), `plan` (warnings) and the bootstrap use case.

Related: [data dictionary](../../../specs/archive/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/archive/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
