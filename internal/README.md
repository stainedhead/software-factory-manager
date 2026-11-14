# internal

Application code, layered per Clean Architecture; dependencies point inward only.

| Directory | Layer |
|---|---|
| [domain/](domain/) | Entities, value objects and domain errors; no outward imports |
| [usecase/](usecase/) | Application logic and the ports (interfaces) it consumes; imports domain only |
| [testkit/](testkit/) | Test-only helpers (fakecmd, invocation-log assertions); imported only by `_test.go` files |

`adapter/` (implementations of the ports) is added as the adapters land. Used by [../cmd/](../cmd/). Back to the [root README](../README.md).
