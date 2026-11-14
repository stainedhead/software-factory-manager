# scripts/archcheck

`make archcheck` pipes `go list` (each package with its production and test imports) into this program, which exits 1 naming each broken rule:

| Rule | Fails when |
|---|---|
| `domain-imports-outer-layer` | `internal/domain/...` imports a package of this module outside `domain` |
| `domain-imports-third-party` | `internal/domain/...` imports anything outside the standard library and this module |
| `usecase-imports-outer-layer` | `internal/usecase/...` imports `adapter`, `cmd` or `testkit` |
| `front-ends-import-each-other` | `adapter/tui` imports `adapter/cli`, or the reverse |

| File | Contents |
|---|---|
| `main.go` | `checkReader` (parses the `go list` lines), `check` (the rules), `report` (prints and returns the exit code) |
| `main_test.go` | One seeded violation per rule, an allowed import list, and de-duplication |

Standard library only. Review FR-010c. Back to [scripts](../README.md).
