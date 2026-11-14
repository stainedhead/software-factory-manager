# internal/adapter/brew/testdata

`brew info --json=v2`, `brew tap` and `brew trust` outputs used by the brew adapter tests.

**Provenance.** `info-installed-real.json` is captured from `brew info --json=v2 --installed` on the macOS CI runner (**Homebrew 7.0.11**, 2026-11-14, trimmed to five formulae and all five installed casks). The other files were written by hand, and `info-installed.json` was then corrected to the real Homebrew 7 shape (installed casks carry an absolute `target` next to each stanza). `TestInfoShape` checks that every file here has the JSON types facman reads, and `TestIntegrationBrewContract` (`-tags integration`) checks the same against the real Homebrew on macOS CI and can capture real files with `FACMAN_CAPTURE_DIR` (see [user-docs/field-check.md](../../../../user-docs/field-check.md#part-3-capture-real-brew-output-for-the-test-fixtures)). No Homebrew was available where workstream D was built, so these files follow the shape documented in [research.md RQ-1 and RQ-2](../../../../specs/archive/261114-software-factory-manager/research.md#rq-1-brew-info---jsonv2) (JSON v2 fields `name`, `full_name`, `tap`, `installed[].version`, `linked_keg`, `keg_only`; cask `token`, `full_token`, `installed`, `artifacts` with `app`, `binary` plain and with `target`, `pkg`, `installer`). The macOS CI contract test confirmed the formula fields and found the cask `target` difference on Homebrew 7.0.11; the uninstalled-cask, `brew tap` and `brew trust` files are still hand-written. The `brew trust` listing format in `trust-list.txt` is a guess (R-OQ-1); the parser treats anything it cannot read as "not trusted", which only adds a reviewed step.

| File | Content |
|---|---|
| `info-installed-real.json` | Real Homebrew 7.0.11 output: `azure-cli` (binary and completions with targets), `firefox` (app and `command_wrapper`), `google-chrome`, `microsoft-edge`, `session-manager-plugin` (`pkg` plus a binary target); formulae `gh`, `git`, `jq`, keg-only `node@24` and `sqlite` |
| `info-installed.json` | `brew info --json=v2 --installed`: formulae with a linked keg, keg-only, unlinked; casks with `binary` and `app` stanzas with Homebrew 7 sibling `target`s, a tap cask |
| `info-empty.json` | Nothing installed |
| `info-cask-pkg.json` | `brew info --json=v2 --cask …` for casks with `pkg`, `installer` and `app` with `target` |
| `info-formula-kegonly.json` | `brew info --json=v2 --formula sqlite` (keg-only, not installed) |
| `tap-list.txt`, `trust-list.txt` | `brew tap` and `brew trust` with no arguments |

Back to [brew](../README.md).
