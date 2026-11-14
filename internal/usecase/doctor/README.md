# internal/usecase/doctor

Whole-catalog conflict and tap-trust check (T-E8). Ports: `CatalogSource` (kept for the data dictionary; the caller passes the loaded catalog, see I-26), `TapChecker`, `ConflictChecker` (satisfied by `usecase/conflicts.Service`).

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `doctor.go` | `Service.Run(ctx, Input{Catalog, Platform, Statuses, Findings})` → `Report{Findings, Conflicts, Checked}`: every catalog brew item available here is pre-flighted (not only selected ones) → `conflict` warning with the fix hint; a tap item needing item trust that is not trusted while its tap is added or it is installed through brew → `untrusted-tap` warning with the exact `brew trust --cask|--formula <item>` fix; a tap not added yet → `tap-not-added` info; a check that cannot finish → `check-failed` info. `Report.Err` is exit 2 when any finding is a warning, nil when clean or info only |
| `doctor_test.go` | Clean, info only, conflict, untrusted tap (added or installed), unreadable tap state, bootstrap warnings, finding texts |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
