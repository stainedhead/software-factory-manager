# internal/adapter/tui

The interactive driving adapter: a Bubble Tea v2 program (`charm.land/bubbletea/v2`) that walks **Scan → Plan → Review → Run → Summary**, install only (FR-026, PRD Appendix C). `facman` with no command opens it when stdout and stdin are terminals. The TUI holds no business logic. Every step is a call on the session ([../session/](../session/)), which composes the use cases. The TUI only turns keys into a selection, shows what the session found and planned, and hands the terminal to sudo commands.

| File | Contents |
|---|---|
| `tui.go` | `Runner` and its `Request` (cmd/facman adapts it to `cli.TUIRunner` through `cli.TUIFunc`; `adapter/tui` never imports `adapter/cli`, review FR-010b), `Options`, the `Engine`/`Steps` ports (`SessionEngine` adapts `*session.Session`; `*session.Run` is the `Steps`), the `bridge` that carries run events, `progress` (`execute.Progress`), `handoffRunner` (`execute.CommandRunner`) and `consent` (`execute.Confirmer`) |
| `model.go` | `Model`: screens, keys, the step commands (start, plan, run), Ctrl+C during Run and the sudo hand-off through `tea.ExecProcess` |
| `rows.go` | The Plan table: rows grouped by ownership state, choice groups as single-select rows, what can be ticked and why not, the filter and the cursor; `toggle` turns space into a `catalog.Selection` |
| `view.go` | Rendering of each screen, the help overlay, the `i` info panel and the conflict evidence; lines are cut to the terminal width with `…` |
| `*_test.go`, `testdata/*.golden` | Table-driven `Update`/`View` tests over fake sessions (the real planning and execute use cases behind canned scan results), golden screens, a real `tea.Program` over pipes for the first-frame budget (< 500 ms) and the `tea.ExecProcess` hand-off |

## Screens and keys

| Screen | What it shows | Keys |
|---|---|---|
| Scan | Machine, Homebrew and Node, counts by ownership state, info findings (superseded tools, untrusted taps). A failed start (lock held, fatal finding, bad profile) shows the error and its fix | `enter` Plan |
| Plan | Wanted packages grouped by state. Required missing ones start ticked, optional ones do not. `foreign`, `duplicate`, `managed` and the like have no checkbox (adopt arrives in Increment 2). `conflict`, `unavailable-here` and incomplete detections are greyed `[-]` with the reason. Choice groups are `( )` single-select, or none. Helpers pulled in by `requires` show "needed by …" | `↑`/`↓` `k`/`j`, `space` tick, `i` info (source, why, which profile, detections, conflict evidence), `/` filter, `enter` Review, `esc` Scan |
| Review | Every exact command in order, with sudo, unverified-script, tap, item-level trust and "after N" marks, the warnings, what is not installed and why, and the OS user consent is recorded for. An empty plan says "Nothing to install." | `enter` consents (the only key that does), `esc` Plan |
| Run | Each action with a spinner, then ok/FAILED/skip; the log tail; who has the terminal during a hand-off | `ctrl+c` asks, then stops after the current command; `q` ignored |
| Summary | One line per package, "installed, sign-in required" next steps, shell-profile lines, log paths, and the exit reason | `esc` back to Review only when nothing ran, `q` quit |

Everywhere outside Run: `?` help, `tab` says Manage and Files arrive later, `q` and `ctrl+c` quit with nothing changed.

## How it stays an adapter

- **Selection.** Ticks become `catalog.Selection{Profiles, With, Without}`. Every toggle is resolved again by `Steps.Resolve` (pure), so `requires` and choice picks follow the domain rules. Enter on Plan calls `Steps.PlanSelection`, which runs the conflict pre-flight and the planner.
- **Consent.** Enter on Review stores the reviewed plan in `consent`. `execute` asks it, and it agrees only to a plan with exactly those actions and commands. `execute` records `user@time` on every operations row.
- **Run.** `Steps.Execute` runs in a goroutine. Its progress, the hand-off requests and the final result travel on the `bridge` channel in order, and the model reads them one at a time. The view reads only the result copy the model took when a step finished, never the session while a step runs.
- **Sudo hand-off (AD-16, R-4).** `handoffRunner` sends interactive commands to the model as `execRequestMsg` and blocks. The model builds the `exec.Cmd` with the injected `Handoff` (`execrunner.Runner.Handoff`: same env, PATH lookup and log as the runner) and returns `tea.ExecProcess`. Its callback writes the exit code to the reply channel. `facman` never sees the password.
- **Quit.** `Runner.Run` ends the program, cancels a run in progress (it stops after the current command), waits for it and closes the session, which releases the state lock.

**Architecture.** Driving adapter; imports `adapter/cli` (the `TUIRunner` seam), `adapter/session`, `domain/...`, `usecase/execute` and `usecase/process` (port types), Bubble Tea v2 and Lip Gloss v2. Wired in [cmd/facman](../../../cmd/facman/). Golden files: `go test ./internal/adapter/tui -update`. Back to [internal/adapter](../README.md).
