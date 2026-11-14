# internal/domain/appinfo

The single source for the application's identity: `facman`, the **Agentic Software Factory Manager**, built from the `software-factory-manager` repo.

| Item | Purpose |
|---|---|
| `Name`, `Title`, `Repository`, `Description` | Identity constants; use these instead of hard-coding the name |
| `VersionLine(version)` | One line printed by `facman --version` |
| `About(version)` | Multi-line text printed by `facman --about` (and when run with no arguments) |

No dependencies. Used by [`cmd/facman`](../../../cmd/facman/). The version string is injected at build time (`make build` sets it from `git describe`). Back to [domain](../README.md).
