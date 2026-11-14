# internal/testkit/fakeworld

**Test-only.** The behaviour behind `fakecmd`: a `World` (JSON file next to the fake links) of `Rule`s that answer any command with fixed stdout, stderr, exit code and an optional sleep, plus an emulated Homebrew (`Brew`) used when no rule matches. The emulation supports `brew --prefix`, `--version`, `shellenv`, `info --json=v2` (`--installed`, `--cask <id>`, formula names), `install [--cask]` (tap items need their tap and item trust, like Homebrew 6), `tap` and `trust` (list and add). Installs update the world, so later `brew info` reflects them, and can create files and symlinks under the prefix.

| File | Contents |
|---|---|
| `world.go` | `World`, `Rule`, `Brew`, `Formula`, `Cask`, `Installed`, `Invocation`, `Result`, `Load`/`Save`, `AppendLog`/`ReadLog`, `Match`, `Builtin` |

Every call is appended to `fakecmd-log.jsonl` as `{argv, env_names, cwd}` (variable names only, never values). Used by [../../../testdata/fakecmd](../../../testdata/fakecmd/) and [..](../README.md).
