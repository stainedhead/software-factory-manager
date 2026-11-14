# scripts/doccheck

`make doccheck` runs this program from the repository root. It exits 1 naming each problem:

- a `.PHONY` target of the `Makefile` with no `` | `make <target>` `` row in the Commands table of [AGENTS.md](../../AGENTS.md#commands) (review FR-015);
- a guide in `FutureGuides` (the user guides of features after Increment 1) without `**Status: planned for Increment N.** … exit(s) 64 until then` in its first three lines (FR-018). Remove a guide from the list when its feature ships.

| File | Contents |
|---|---|
| `main.go` | `check(fs.FS)`, `commandsTable`, `commandProblems`, `statusProblem`, `report` |
| `main_test.go` | Table-driven cases over an in-memory file system (`testing/fstest`) |

Standard library only. Back to [scripts](../README.md).
