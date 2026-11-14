# internal/usecase/process

Value types of the `CommandRunner` port: `Cmd` (argv, env overrides, dir, interactive, log writer, timeout) and `Result`, plus `ErrTimeout` and `ErrNotFound`. Each use case that runs commands declares its own `CommandRunner` over these types, so `adapter/execrunner` satisfies all of them.

| File | Contents |
|---|---|
| `process.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/archive/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
