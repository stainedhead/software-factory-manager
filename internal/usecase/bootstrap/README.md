# internal/usecase/bootstrap

Startup bootstrap check (T-E2). Ports: `CommandRunner`, `PlatformProbe`, `HomebrewLocator`, `PathLookup`.

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `bootstrap.go` | `Service.Check(ctx)` → `platform.BootstrapReport`: UID 0 (fatal, stops the check), Xcode CLT (`xcode-select -p`), Linux prerequisites (gcc, ps, curl, file, git → exact apt/dnf command; `BlocksApply`), glibc below 2.39 or unknown (warning), headless Linux (info), Homebrew (missing → warning, not on PATH → info, prefix of another user → `BlocksApply`, `shellenv` read), inherited `HOMEBREW_*` names from `Service.Environ` into `HomebrewInfo.InheritedEnv` and a `homebrew-cask-opts` warning for `HOMEBREW_CASK_OPTS` with `--no-quarantine` (review FR-013), Node/npm (version, provenance, ≥ 24.11; keg-only `node@24` found under the prefix). `ChildEnv(report, basePath)`: shellenv plus `{prefix}/opt/node@24/bin` first on `PATH`, for every child (D-3) |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/archive/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
