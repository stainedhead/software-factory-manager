# internal/adapter

Implementations of the use case ports: subprocesses, filesystem, SQLite, YAML, HTTP, and the driving adapters (CLI, TUI). Adapters may import `internal/domain/...` and `internal/usecase/...`; nothing inward imports an adapter. Each adapter asserts at compile time that it satisfies its ports.

| Package | Purpose | Ports implemented |
|---|---|---|
| [catalogyaml/](catalogyaml/) | Embedded catalog v1 and profiles; `--catalog <dir>` | `scan`/`planning`/`doctor.CatalogSource` |
| [execrunner/](execrunner/) | Subprocesses (argv only, env sanitizing, timeouts, log tee, interactive), system clock and user | `bootstrap`/`scan`/`execute.CommandRunner`, `bootstrap.PathLookup`, `execute.Clock`, `execute.UserInfo` |
| [platform/](platform/) | OS, arch, WSL, display, glibc, distro, UID probe | `bootstrap.PlatformProbe` |
| [statedir/](statedir/) | State directory paths, log names and pruning, exclusive lock | `scan.StateDir`, `execute.StateDir` |
| [sqlite/](sqlite/) | Inventory database and migrations | `scan`/`execute`/`status.Inventory` |

More adapters (`brew`, `detect`, `fsprobe`, `installer`, `report`, `cli`, `tui`) are added by their workstreams. Wired in [../../cmd/facman/](../../cmd/facman/). Back to [internal](../README.md).
