# internal/adapter/cli

The command-line driving adapter: the cobra command tree, flag validation, the headless `[y/N]` prompt, progress and text rendering, `--json` output, exit codes and signal handling.

| File | Contents |
|---|---|
| `cli.go` | `App{Engine, TUI, Stdio, Version, Now, Signals}` and `Execute(ctx, args) int`; subcommands `scan`, `plan`, `apply`, `status`, `doctor` (hidden deferred commands exit 64); `Engine` and the `TUIRunner` seam |
| `normalize.go` | Exact-token short-flag normalizer (`-cl`, `-hs`, `-ps`, `-os`, `-ts`, `-nop`) and the deferred flags that exit 64 naming their increment (FR-005, PRD Appendix B) |
| `prompt.go` | `Prompt.Confirm`: prints the plan, then `Proceed? [y/N] `; only `y`/`yes` consents; Ctrl+C declines |
| `progress.go` | Line-based progress for headless runs |
| `render.go` | Text output for each command: findings, the package table, the plan, the apply summary |
| `*_test.go` | Exit-code precedence, usage errors never reach the engine, flag collisions, prompt answers, pty tests for the root command, rendering, JSON |

- **Exit codes.** `failure.ExitCode`: usage 64 > bootstrap 4 > validation 3 > operation 1 (drift 2). Flag errors and any untyped cobra error are usage errors.
- **Validation before I/O.** `--catalog` must be a local directory, `--profile` / `--with` values are trimmed and de-duplicated, and `apply` without `--dry-run` needs a terminal on stdin (else 64).
- **`--json`.** The report (schema v1, [../report/](../report/)) goes to stdout; the prompt and progress go to stderr.
- **Signals.** The first SIGINT/SIGTERM cancels the run (execute stops after the current command); later ones are ignored until it returns.
- **TUI seam.** `facman` with no command calls `TUIRunner` ([../tui/](../tui/)) when stdout and stdin are terminals. Stdout not a terminal prints the about text and a hint (exit 0, AD-14). Stdout a terminal but stdin not is a usage error (64): keys, and with them consent, only come from a terminal.

**Architecture.** Adapter; imports `adapter/session` (requests and results), `adapter/report`, `adapter/catalogyaml` (directory check), `domain/...` and cobra. Wired in [cmd/facman](../../../cmd/facman/). Back to [internal/adapter](../README.md).
