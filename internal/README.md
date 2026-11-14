# internal

Application code, layered per Clean Architecture; dependencies point inward only.

| Directory | Layer |
|---|---|
| [domain/](domain/) | Entities, value objects and domain errors; no outward imports |
| [usecase/](usecase/) | Application logic and the ports (interfaces) it consumes; imports domain only |
| [adapter/](adapter/) | Implementations of the ports (YAML, subprocesses, filesystem, SQLite, CLI, TUI) |
| [testkit/](testkit/) | Test-only helpers (fakecmd, invocation-log assertions); imported only by `_test.go` files |

Used by [../cmd/](../cmd/). Back to the [root README](../README.md).
