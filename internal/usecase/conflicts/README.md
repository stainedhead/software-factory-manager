# internal/usecase/conflicts

Link-path conflict pre-flight for brew items (T-E4). Ports: `Backend` (`ID`, `LinkPaths`), `PathProbe` (`Inspect`, `Owner`).

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `conflicts.go` | `Service.Check(ctx, entry, step)` → first `*plan.Conflict` (path, owner class and detail, evidence, failing command, fix hint) or nil; paths held by this package are fine. `CheckAll` reports unreadable items in `Failed` (they must not be planned). `InstallArgv`, `FixHint` |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
