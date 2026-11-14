# internal/adapter/brew

The Homebrew adapter. Everything facman asks of `brew` goes through here, so the exact argv lives in one place.

| File | Contents |
|---|---|
| `locate.go` | `Locator` (`bootstrap.HomebrewLocator`): brew on PATH (absolute entries only), else at the known prefixes (`/opt/homebrew`, `/usr/local`, `/home/linuxbrew/.linuxbrew`; injectable), prefix owner check, `brew --version`; `ShellEnv` runs `brew shellenv bash` and `ParseShellEnv` reads its `export` lines without a shell (`$X`, `${X+..}`, `${X-..}`, `${X:-..}`, `${X#..}`; anything else is skipped); `ReadOnlyEnv` (`HOMEBREW_NO_AUTO_UPDATE=1`, `HOMEBREW_NO_ENV_HINTS=1`, D-15) |
| `info.go` | `ParseInfo` for `brew info --json=v2` (formula version, keg-only, linked keg; cask version, `app`/`binary` targets, `pkg`/`installer` → needs sudo); `ParseTaps`, `ParseTrusted` (`brew tap`, `brew trust` listings; only full `user/repo/name` items count as trusted) |
| `info_shape_test.go`, `contract_integration_test.go` | `infoShapeProblems` checks the JSON types of every `brew info` field facman reads, on the fixtures (`TestInfoShape`) and, with `-tags integration`, on the real Homebrew (`TestIntegrationBrewContract`: `--installed`, `hello`, keg-only `sqlite` and the `zoom` cask on macOS, plus `brew tap` and `brew trust`; read-only; `FACMAN_CAPTURE_DIR` saves the outputs) |
| `detector.go` | `Detector` (`scan.Detector`): one `brew info --json=v2 --installed` call (10 s), installed formulae and casks mapped to entries by install id (tap items by full name, so `orca` never matches `stablyai/orca/orca`); `Ref` (`brew-cask:claude-code`) |
| `backend.go` | `Backend` (`NewFormula`, `NewCask`; `planning.Backend`, `conflicts.Backend`, `doctor.TapChecker`): `InstallActions` (`brew install [--cask] <id>`; for tap items separate `brew tap <tap>` and `brew trust --cask <item>` actions, each omitted when `TapState` says it is in place), `LinkPaths`, `TapState`, `InstallCommand` |
| `testdata/` | `brew info`, `brew tap` and `brew trust` fixtures (shaped from research.md, **not yet captured from a real Homebrew**; see its README) |

Rules kept here:

- Commands are argv only, with `brew` as `Argv[0]` (found on the child PATH, which the use cases set from `shellenv`). No command contains `--force`, `--overwrite`, `link`, `upgrade` or `uninstall`, and trust is item-level only.
- Read-only calls (`info`, `tap`, `trust` listings, `shellenv`, `--version`) set `ReadOnlyEnv` and a 10 s timeout. Planning never mutates.
- A cask's install command needs sudo (terminal handed over) when its artifacts include `pkg` or `installer`, or when they cannot be read yet (Homebrew missing, tap not added): the safe default.
- `LinkPaths`: cask `binary` targets under `{prefix}/bin` and `app` bundles under `/Applications` (injectable); a formula's `detect.commands` under `{prefix}/bin`, or none when keg-only; a tap item whose tap is not added yet falls back to `detect.commands` (D-9); with Homebrew missing, nothing (execute re-checks after the bootstrap).
- `Target` (path, prefix, child env) is read through a function because it changes when facman installs Homebrew mid-run.

**Architecture.** Adapter: imports domain `catalog`, `plan`, `ownership`, `platform` and the port packages it satisfies (`usecase/bootstrap`, `scan`, `planning`, `conflicts`, `doctor`, `process`). Wired in `cmd/facman`. Related: [research.md RQ-1, RQ-2](../../../specs/archive/261114-software-factory-manager/research.md), [design §3b](../../../docs/design/software-factory-installer-design.md). Back to [internal/adapter](../README.md).
