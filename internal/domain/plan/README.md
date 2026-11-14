# internal/domain/plan

What facman intends to do: `Command` (exact argv), `Action`, `Plan`, `Exclusion`, `Conflict` with `ConflictEvidence`, `TapTrustStep` and the `Consent` value object (`user@RFC3339-UTC`). `Plan.Validate` enforces the safety invariants: no short or long form of `--force`/`--overwrite` (`-f`, `-fsSL`, `--forc`), no option after a brew subcommand except `--cask`/`--formula` on `install` and `trust`, brew subcommands limited to `install`, `tap` and `trust`, item-level trust only, no `HOMEBREW_NO_REQUIRE_TAP_TRUST*`, actions only for `absent` packages.

| File | Contents |
|---|---|
| `plan.go` | Types and `Consent`; `Command.String` quotes and escapes control and non-printable characters so data cannot hide text on a consent screen |
| `order.go` | `OrderPackages` (stable topological order by requires, ties by catalog order), `ExcludeWithDependents` |
| `validate.go` | `Plan.Validate`, `ErrInvariant` |

**Architecture.** Domain layer: no I/O. Imports `catalog`, `platform`. Used by `inventory` (commands in operations), `ownership` (conflicts) and the planning and execute use cases.

Related: [data dictionary](../../../specs/archive/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/archive/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
