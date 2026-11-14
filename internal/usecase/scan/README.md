# internal/usecase/scan

Parallel detection, inventory reconcile and classification (T-E3). Ports: `Detector` (with the detection `Env`), `Inventory` (records, upsert, mark interrupted, save detections), `CatalogSource`, `StateDir` (`TryLock`), `CommandRunner`. `DetectorTimeout` is 10 s.

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `scan.go` | `Service.Run(ctx, Input)` → `Result`: takes the lock with `TryLock` unless the caller holds it (`HoldsLock`); with the lock, `MarkInterrupted` and AD-8 reconcile (an interrupted install now detected through its backend → `managed`; an interrupted Homebrew bootstrap with brew now found → `homebrew` record); detectors in parallel, each with its timeout, seeing only the entries they `Covers`; a failed or timed-out detector → `unknown` detections for its entries; `Classify` for every catalog entry in catalog order; `superseded` info finding for a found detection-only entry (Gemini CLI); detection snapshot saved. Without the lock: no writes and a `state-locked` warning. `ReadOnly` (a dry run): no lock, no writes, no warning. `Result.ApplyConflicts` marks absent packages with a pre-flight conflict as `conflict` A `path-relative-entry` warning is added when PATH has a relative or empty entry (review FR-002). |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/archive/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
