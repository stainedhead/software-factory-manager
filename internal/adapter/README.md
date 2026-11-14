# internal/adapter

Implementations of the use case ports: subprocesses, filesystem, SQLite, YAML, HTTP, and the driving adapters (CLI, TUI). Adapters may import `internal/domain/...` and `internal/usecase/...`; nothing inward imports an adapter. Each adapter asserts at compile time that it satisfies its ports.

| Package | Purpose | Ports implemented |
|---|---|---|
| [catalogyaml/](catalogyaml/) | Embedded catalog v1 and profiles; `--catalog <dir>` | `scan`/`planning`/`doctor.CatalogSource` |

More adapters (`execrunner`, `platform`, `statedir`, `sqlite`, `brew`, `detect`, `fsprobe`, `installer`, `report`, `cli`, `tui`) are added by their workstreams. Wired in [../../cmd/facman/](../../cmd/facman/). Back to [internal](../README.md).
