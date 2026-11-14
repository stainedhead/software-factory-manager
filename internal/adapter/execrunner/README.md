# internal/adapter/execrunner

Runs subprocesses for every use case (AD-7) and provides the system clock and user.

| File | Contents |
|---|---|
| `runner.go` | `Runner` (`New(Options{BaseEnv, Stdin, Stdout, Stderr})`), `Run(ctx, process.Cmd)`, `Environ(overrides)`, `LookPath(name)` |
| `system.go` | `SystemClock` (`execute.Clock`), `SystemUser` (`execute.UserInfo`: `os/user`, falling back to `$USER`) |

Behaviour:

- **Argv only.** `Argv[0]` is looked up on the child `PATH` (the command's `PATH` override if set, else the base `PATH`); nothing goes through a shell.
- **Environment.** Base (usually `os.Environ()`) plus `Cmd.Env` overrides, with every `HOMEBREW_NO_REQUIRE_TAP_TRUST*` variable removed (FR-047, I1-10).
- **Log.** When `Cmd.Log` is set: a header `$ "argv"…` and `# env: NAMES` (override names only, never values), then stdout and stderr as they arrive, then `# exit N`.
- **Timeouts and cancellation.** `Cmd.Timeout` kills the child and returns an error wrapping `process.ErrTimeout`; a cancelled ctx kills the child and returns the ctx error. Callers that must let a command finish (installs: "stop after the current command") pass a context that is not cancelled, such as `context.WithoutCancel`.
- **Interactive.** `Cmd.Interactive` gives the child the parent's stdin, stdout and stderr (sudo prompts); output is not captured, and facman never sees a password.
- A non-zero exit is a `Result`, not an error; a missing program wraps `process.ErrNotFound`.

**Architecture.** Adapter implementing `bootstrap`/`scan`/`execute.CommandRunner` and `bootstrap.PathLookup`; imports `usecase/process` and domain `plan`. Wired in `cmd/facman`; wrapped by `adapter/tui` for `tea.ExecProcess` (AD-16). Back to [internal/adapter](../README.md).
