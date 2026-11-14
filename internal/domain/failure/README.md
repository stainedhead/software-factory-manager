# internal/domain/failure

Typed errors that decide exit codes (PRD Appendix H): usage 64, validation 3, bootstrap 4, operation 1, drift 2. `Worst` picks the error that decides the exit code (usage > bootstrap > validation > operation > drift).

| File | Contents |
|---|---|
| `failure.go` | `Kind`, `Error` (with `Fix` and `Details`), constructors `Usage`, `Validation`, `Bootstrap`, `Operation`, `Drift`, `ExitCode`, `KindOf`, `Worst` |

**Architecture.** Domain layer: no I/O. Imports nothing. Used by `catalog` and by every use case and driving adapter that maps errors to exit codes.

Related: [data dictionary](../../../specs/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
