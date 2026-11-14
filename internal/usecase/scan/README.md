# internal/usecase/scan

Parallel detection, inventory reconcile and classification (T-E3). Ports: `Detector` (with the detection `Env`), `Inventory` (records, upsert, mark interrupted, save detections), `CatalogSource`, `StateDir` (`TryLock`), `CommandRunner`. `DetectorTimeout` is 10 s.

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
