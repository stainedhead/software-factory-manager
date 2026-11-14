# internal/adapter/session

Composes the use cases into one run of a `facman` command: bootstrap check, state, catalog, scan, then plan and execute, status or doctor. The CLI (and later the TUI) drives a `Session`; `cmd/facman` wires its `Deps`.

| File | Contents |
|---|---|
| `session.go` | `Deps` (ports and environment facts), `New`, `Session.Run(ctx, Request)` and the step-by-step `Start` / `Run.Plan` / `Run.Execute` / `Run.Status` / `Run.Doctor`; `Replanner` (the `execute.Replanner` port); `Machine` (the located brew, child env and npm prefix, shared with the backends) |
| `session_test.go` | Fakes for every port: each command, each failure kind and its exit code, lock refusal, dry run writes nothing, consent identity, the replanner after a bootstrap |

- **Order and exit codes.** Bootstrap check and fatal findings (4), state directory and lock (4), catalog and profile resolution (3), then the command. A dry run is judged like `plan`, so blocking findings do not stop it.
- **The lock.** `execute` does not lock. A mutating request takes the state lock before the scan and keeps it until the run closes; a second mutating run gets a bootstrap error ("another facman (PID n) is changing this machine", exit 4). Read-only requests and dry runs scan with `ReadOnly` and write nothing.
- **Preflight.** Before planning, absent brew items are checked for conflicts; the plan refuses them (FR-034).
- **Replanner.** After `bootstrap-homebrew` runs, it re-checks bootstrap, updates the `Machine`, re-scans under the held lock and returns what is now present, incomplete or conflicting.
- **Consent.** `execute` records the OS user and the time (`user@RFC3339`).

**Architecture.** Adapter that composes `usecase/bootstrap`, `scan`, `conflicts`, `planning`, `execute`, `status`, `doctor` and `summary`; imports `domain/...`. It holds no adapters itself: they come in through `Deps`. Used by [../cli/](../cli/); wired in [cmd/facman](../../../cmd/facman/). Back to [internal/adapter](../README.md).
