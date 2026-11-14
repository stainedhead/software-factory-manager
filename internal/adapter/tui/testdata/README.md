# internal/adapter/tui/testdata

Golden screens for the TUI tests, rendered at a fixed 100×60 size with ANSI stripped (implementation-notes I-51).

| Files | Screen |
|---|---|
| `scan.golden`, `plan.golden`, `plan-info.golden`, `review.golden`, `help-review.golden`, `run.golden` | Single screens and overlays |
| `flow-1-scan.golden` to `flow-4-summary.golden` | One full Scan → Plan → Review → Summary flow |

Regenerate with `go test ./internal/adapter/tui -update` after an intended UI change, and review the diff. Back to [tui](../README.md).
