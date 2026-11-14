# internal/domain/inventory

What facman keeps about what it installed: `Record` (only `managed` or `adopted` are stored), the append-only `Operation` log with its status transitions (`in_progress` → `succeeded`/`failed`/`interrupted`, nothing out of a terminal state), `DetectionSnapshot` and `TapTrustGrant`.

| File | Contents |
|---|---|
| `inventory.go` | Types, `Record.Validate`, `CanTransition`, `Operation.Finish`, `OpFor`, `HomebrewRef`, `InstallRef` (backend and `backend:item` ref from a recorded `brew install [--cask]` command; shared by scan reconcile and execute) |

**Architecture.** Domain layer: no I/O. Imports `catalog`, `plan`. Used by `ownership` and by `adapter/sqlite`, which stores these types (data-dictionary §4).

Related: [data dictionary](../../../specs/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
