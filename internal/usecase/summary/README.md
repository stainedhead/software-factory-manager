# internal/usecase/summary

End-of-run summary for `apply` and the TUI Summary screen (T-E9, FR-112, FR-113). No ports: the caller maps `execute.Outcome` into `Input` (`Results` with the execute status strings), so the package imports the domain only.

| File | Contents |
|---|---|
| `summary.go` | `Build(Input{Catalog, Plan, Results, Homebrew, BrewPrefix, BrewInstalled})` → `Summary{Lines, NextSteps, ProfileLines}`. One `Line` per package in plan order, then the remaining exclusions: `installed`, `installed, sign-in required`, `already present`, `not installed: failed / dependency failed / cancelled / conflict / detection incomplete / unavailable here / not selected` with the reason in `Detail` and the deciding log path. Sign-in next steps for installed `sign_in_required` packages. Profile lines: `eval "$(<prefix>/bin/brew shellenv)"` when Homebrew was installed now or is not on PATH, `export PATH="<prefix>/opt/node@24/bin:$PATH"` when node@24 was installed now |
| `summary_test.go` | Full run with every line kind, profile line cases, sign-in without a command |

**Architecture.** Use case layer: imports `internal/domain/...` only. Used by `adapter/cli` and `adapter/tui` (workstreams F and G).

Related: [tasks](../../../specs/archive/261114-software-factory-manager/tasks.md), [product details](../../../docs/product-details.md). Back to [internal/usecase](../README.md).
