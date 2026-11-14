# scripts

Small Go programs that check the repository itself. They are run through the `Makefile` (never by hand), are part of `make check` and CI, and are never shipped in the `facman` binary.

| Folder | Make target | Checks |
|---|---|---|
| [archcheck/](archcheck/) | `make archcheck` | The Clean Architecture import rules in [AGENTS.md](../AGENTS.md#architecture--clean-architecture) (review FR-010c) |
| [doccheck/](doccheck/) | `make doccheck` | Every `.PHONY` target has a row in AGENTS.md's Commands table (FR-015), and every later-increment user guide carries its status note (FR-018) |

**Architecture.** Outside the layers: they import only the standard library, and nothing imports them. Each has table-driven tests that `make test` runs. Back to the [root README](../README.md).
