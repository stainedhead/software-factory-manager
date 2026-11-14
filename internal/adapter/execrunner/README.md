# internal/adapter/execrunner

Runs subprocesses for every use case (AD-7) and provides the system clock and user.

| File | Contents |
|---|---|
| `runner.go` | `Runner` (`New(Options{BaseEnv, Stdin, Stdout, Stderr})`), `Run(ctx, process.Cmd)`, `Handoff(process.Cmd)`, `Environ(overrides)`, `LookPath(name)` |
| `system.go` | `SystemClock` (`execute.Clock`), `SystemUser` (`execute.UserInfo`: `os/user`, falling back to `$USER`) |

Behaviour:

- **Argv only.** `Argv[0]` is looked up on the child `PATH` (the command's `PATH` override if set, else the base `PATH`); nothing goes through a shell.
- **Environment.** Base (usually `os.Environ()`) plus `Cmd.Env` overrides, with every `HOMEBREW_NO_REQUIRE_TAP_TRUST*` variable removed (FR-047, I1-10).
- **Log.** When `Cmd.Log` is set: a header `$ "argv"…` and `# env: NAMES` (override names only, never values), then stdout and stderr as they arrive, then a footer: `# result: exit N` (or `killed by signal NAME (exit 128+signo)`, `timed out after D; killed`, `cancelled; killed`, `error: …`) and `# time: started …, ended …, took …`. A signal-killed child gets `Result.ExitCode = 128+signo` and `Result.Signal` set.
- **Timeouts and cancellation.** `Cmd.Timeout` kills the child and returns an error wrapping `process.ErrTimeout`; a cancelled ctx kills the child and returns the ctx error. Callers that must let a command finish (installs: "stop after the current command") pass a context that is not cancelled, such as `context.WithoutCancel`.
- **Process groups.** A non-interactive child runs in its own process group (`Setpgid`), so a `Ctrl+C` at the terminal reaches facman (which stops after the current command) but never kills the child midway. An interactive child stays in the foreground group because it owns the terminal.
- **Interactive.** `Cmd.Interactive` gives the child the parent's stdin, stdout and stderr (sudo prompts); output is not captured, and facman never sees a password.
- **Hand-off.** `Handoff(cmd)` prepares an interactive command for a front end that hands the terminal over itself (the TUI's `tea.ExecProcess`): the `exec.Cmd` gets the same environment and `PATH` lookup as `Run`, stdio is left to the front end, the log header is written, and the returned `finish(runErr)` turns the outcome into a `Result` and writes the same footer.
- A non-zero exit is a `Result`, not an error; a missing program wraps `process.ErrNotFound`.

**Architecture.** Adapter implementing `bootstrap`/`scan`/`execute.CommandRunner` and `bootstrap.PathLookup`; imports `usecase/process` and domain `plan`. Wired in `cmd/facman`; wrapped by `adapter/tui` for `tea.ExecProcess` (AD-16). Back to [internal/adapter](../README.md).
