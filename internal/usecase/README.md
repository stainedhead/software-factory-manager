# internal/usecase

Application logic. Each package declares the ports (interfaces) it consumes, next to the code that uses them (AD-4); there is no shared ports package. Use cases import `internal/domain/...` only and are tested with in-memory fakes.

| Package | Purpose | Ports declared |
|---|---|---|
| [process/](process/) | `Cmd`/`Result` value types shared by every `CommandRunner` port | — |
| [bootstrap/](bootstrap/) | Startup bootstrap report and fatal decisions | `CommandRunner`, `PlatformProbe`, `HomebrewLocator`, `PathLookup` |
| [scan/](scan/) | Detect, reconcile, classify, persist snapshots | `Detector`, `Inventory`, `CatalogSource`, `StateDir`, `CommandRunner` |
| [conflicts/](conflicts/) | Link-path conflict pre-flight | `Backend`, `PathProbe` |
| [planning/](planning/) | Desired set + statuses → `Plan` | `Backend`, `CatalogSource` |
| [execute/](execute/) | Run a consented plan with the operations log | `CommandRunner`, `Inventory`, `HomebrewInstaller`, `StateDir`, `Clock`, `UserInfo`, `Progress`, `Confirmer`, `Replanner` |
| [doctor/](doctor/) | Whole-catalog conflict and tap-trust check | `CatalogSource`, `TapChecker`, `ConflictChecker` |
| [status/](status/) | Ownership view | `Inventory` |
| [summary/](summary/) | End-of-run summary: per-package lines, sign-in next steps, shell profile lines | — |

No use case imports another (only the shared `process` types); `cmd/facman` composes them, for example the execute `Replanner` over bootstrap, scan and conflicts. Used by [../adapter/](../adapter/) (driving adapters) and wired in [../../cmd/facman/](../../cmd/facman/). Back to [internal](../README.md).
