# internal/usecase/status

Managed/adopted/foreign/drifted/duplicate view (T-E8). Ports: `Inventory` (`Records`, `TapTrustGrants`).

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `status.go` | `Service.Run(ctx, catalog, statuses)` → `Report{Rows, Homebrew, TapTrust}`: managed, adopted, foreign, drifted and duplicate packages in catalog order with provenance, path and version (drift and duplicates explained in `Detail`), then records whose package left the catalog; the Homebrew bootstrap record and the tap-trust grants |
| `status_test.go` | Every shown state, details, records not in the catalog, store errors |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/archive/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
