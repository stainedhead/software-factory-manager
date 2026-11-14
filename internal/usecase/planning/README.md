# internal/usecase/planning

Builds the `Plan` (T-E5). Ports: `Backend` (`ID`, `Available`, `InstallActions`, `LinkPaths`, `TapState`, `Capabilities`), `CatalogSource`.

| File | Contents |
|---|---|
| `ports.go` | Port interfaces (or shared port types), declared here where they are consumed (AD-4) |
| `planning.go` | `Service.Build(ctx, Input)` → `plan.Plan`: selected packages in `plan.OrderPackages` order; absent → backend `InstallActions` (tap → trust → install chained); present in any state → nothing; unavailable-here, conflict, detection-incomplete and dependents of excluded packages → `Excluded` with a reason; a choice group satisfied by any present member drops its pick (and helpers only it needed), a required group without a pick is `not-selected`; Homebrew missing and something planned → `bootstrap-homebrew` (installer, `NeedsSudo`, `Unverified`) is action 1 and every action depends on it; warnings copied from the bootstrap report; `plan.Validate` on every plan (violation = internal error, exit 1) |
| `ports_test.go` | Compile-time checks that in-memory fakes satisfy the ports |

**Architecture.** Use case layer: imports `internal/domain/...` (and `usecase/process` for runner types) only; never an adapter. Adapters implement these ports and carry compile-time assertions such as `var _ scan.Detector = (*PathDetector)(nil)`.

Related: [data dictionary §3](../../../specs/261114-software-factory-manager/data-dictionary.md#3-use-case-ports-interfaces-declared-where-consumed), [tasks](../../../specs/261114-software-factory-manager/tasks.md). Back to [internal/usecase](../README.md).
