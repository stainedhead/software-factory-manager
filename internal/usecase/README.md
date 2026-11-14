# internal/usecase

Application logic. Each package declares the ports (interfaces) it consumes, next to the code that uses them (AD-4); there is no shared ports package. Use cases import `internal/domain/...` only and are tested with in-memory fakes.

| Package | Purpose | Ports declared |
|---|---|---|
| [process/](process/) | `Cmd`/`Result` value types shared by every `CommandRunner` port | — |
| [bootstrap/](bootstrap/) | Startup bootstrap report and fatal decisions | `CommandRunner`, `PlatformProbe`, `HomebrewLocator`, `PathLookup` |
| [scan/](scan/) | Detect, reconcile, classify, persist snapshots | `Detector`, `Inventory`, `CatalogSource`, `StateDir`, `CommandRunner` |
| [conflicts/](conflicts/) | Link-path conflict pre-flight | `Backend`, `PathProbe` |
| [planning/](planning/) | Desired set + statuses → `Plan` | `Backend`, `CatalogSource` |
| [execute/](execute/) | Run a consented plan with the operations log | `CommandRunner`, `Inventory`, `HomebrewInstaller`, `StateDir`, `Clock`, `UserInfo`, `Progress`, `Confirmer` |
| [doctor/](doctor/) | Whole-catalog conflict and tap-trust check | `CatalogSource`, `TapChecker` |
| [status/](status/) | Ownership view | `Inventory` |

The use case code lands in workstream E; `summary/` (no ports) is added with it. Used by [../adapter/](../adapter/) (driving adapters) and wired in [../../cmd/facman/](../../cmd/facman/). Back to [internal](../README.md).
