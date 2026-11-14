# internal

Application code, layered per Clean Architecture; dependencies point inward only.

| Directory | Layer |
|---|---|
| [domain/](domain/) | Entities, value objects and domain errors; no outward imports |

`usecase/` and `adapter/` will be added here as the application grows. Used by [../cmd/](../cmd/). Back to the [root README](../README.md).
