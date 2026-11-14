# internal/adapter/brew/testdata

`brew info --json=v2`, `brew tap` and `brew trust` outputs used by the brew adapter tests.

**Provenance: not captured from a real Homebrew.** No Homebrew was available where workstream D was built, so these files follow the shape documented in [research.md RQ-1 and RQ-2](../../../../specs/archive/261114-software-factory-manager/research.md#rq-1-brew-info---jsonv2) (Homebrew 6 JSON v2 fields `name`, `full_name`, `tap`, `installed[].version`, `linked_keg`, `keg_only`; cask `token`, `full_token`, `installed`, `artifacts` with `app`, `binary` plain and with `target`, `pkg`, `installer`). They must be checked against real output in the I1-16 field run (and I1-14 on macOS CI), and replaced with captured files, noting the Homebrew version, if anything differs. The `brew trust` listing format in `trust-list.txt` is a guess (R-OQ-1); the parser treats anything it cannot read as "not trusted", which only adds a reviewed step.

| File | Content |
|---|---|
| `info-installed.json` | `brew info --json=v2 --installed`: formulae with a linked keg, keg-only, unlinked; casks with `binary`, `app` + `binary` with `target`, a tap cask |
| `info-empty.json` | Nothing installed |
| `info-cask-pkg.json` | `brew info --json=v2 --cask …` for casks with `pkg`, `installer` and `app` with `target` |
| `info-formula-kegonly.json` | `brew info --json=v2 --formula sqlite` (keg-only, not installed) |
| `tap-list.txt`, `trust-list.txt` | `brew tap` and `brew trust` with no arguments |

Back to [brew](../README.md).
