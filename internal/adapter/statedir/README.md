# internal/adapter/statedir

The state directory (PRD Appendix J), implementing `scan.StateDir` and `execute.StateDir`.

| File | Contents |
|---|---|
| `statedir.go` | `Resolve(goos, getenv)`, `Open(root)`, `Dir` with `Root`, `DBPath`, `LockPath`, `LogsPath`, `LogPath`, `PruneLogs`, `TryLock`, `Lock` |

| Item | macOS | Linux / WSL |
|---|---|---|
| State dir | `~/Library/Application Support/facman/` | `${XDG_STATE_HOME:-~/.local/state}/facman/` |
| DB | `state.db` | same |
| Lock | `facman.lock` (exclusive `flock`, holder PID written inside) | same |
| Logs | `logs/<YYYYMMDD-HHMMSS>-<operation id>-<package id>.log` (UTC) | same |

- `HOME` unset or relative, an uncreatable or unwritable directory: bootstrap error (exit 4) naming the variable or path.
- `PruneLogs(now)` deletes regular files in `logs/` older than 90 days and nothing else.
- `TryLock()` never waits: when another process holds the lock it returns a nil unlock and the holder's PID (AD-9, D-10). `Lock(ctx)` waits until ctx is done, then fails with exit 4 naming the PID.

**Architecture.** Adapter; imports domain `appinfo` and `failure`. Back to [internal/adapter](../README.md).
