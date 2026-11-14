# Usage Examples

Worked examples for common tasks with `facman` Increment 1. Each one shows the command and what to expect. Flags and exit codes are described in full in [Configuration](configuration.md). For first-time setup, start with [Getting started](getting-started.md).

## Set up a new workstation

```sh
facman scan                                  # look first; changes nothing
facman                                       # TUI: review the plan, press Enter on Review to install
```

Or headless, in a terminal:

```sh
facman plan  --profile agentic-factory       # read the exact commands
facman apply --profile agentic-factory       # same plan, then Proceed? [y/N]
```

If Homebrew is missing, the first action in the plan installs it. The installer is marked "unverified script: Homebrew publishes no checksum" and gets the terminal so it can ask for your password. After it runs, `facman` re-detects and continues only with the commands you already saw that are still needed.

## Add optional packages

Optional items in `agentic-factory` are off until you ask for them:

```sh
facman plan --profile agentic-factory --with gcloud-cli --with azure-cli
facman apply --profile agentic-factory --with colima      # container runtime: pick one member
```

`colima` brings its `docker` and `docker-compose` helpers with it, ordered first. `--with container-runtime` on its own is exit 3 and lists the members (`orbstack`, `colima`, `docker-desktop`). In the TUI, use `facman --with t3-code`, or tick the item on the Plan screen.

## Combine profiles

```sh
facman plan --profile base-dev --profile agentic-factory
```

The union is wanted. If two profiles pin different versions of one package, the command exits 3 and names both profiles.

## Preview without a terminal

```sh
facman apply --profile base-dev --dry-run    # prints the plan, asks nothing, changes nothing
```

Use this in scripts, CI logs or a pull request comment. A plain `apply` without a terminal on stdin exits 64. Prompt-free installs arrive with configuration lists (`-cl`) in Increment 3.

## Script with `--json`

Every command can write a report (schema `facman.report/v1`) to stdout. With `jq`:

```sh
# Which wanted packages are missing?
facman plan --profile agentic-factory --json \
  | jq -r '.plan.actions[] | "\(.package_id): \(.commands[].argv | join(" "))"'

# Everything installed some other way (facman leaves these alone)
facman scan --json | jq -r '.packages[] | select(.state == "foreign") | "\(.id) \(.provenance) \(.path)"'

# What facman manages, with versions
facman status --json | jq -r '.packages[] | select(.state == "managed") | "\(.id) \(.version // "-")"'

# Did doctor find anything?
facman doctor --json > doctor.json; echo "exit $?"   # 0 clean, 2 needs fixing
jq -r '.findings[] | "\(.severity) \(.package_id // "-"): \(.message)"' doctor.json
```

With `apply --json`, the command list and the `Proceed? [y/N]` prompt go to stderr, and stdout carries only the report, including `results`, `consent_by` and `next_steps`.

## Understand a conflict

```console
$ facman plan --profile agentic-factory
…
Not installed:
  claude-code: conflict; /opt/homebrew/bin/claude is held by orphan (dangling symlink); <fix hint>
```

`facman` will not install over files it does not own, and it never uses `--force`. Ask `doctor` or the TUI's `i` panel for the evidence: the `ls -l` line, the file type, the symlink target, and whether the link is dangling. Then fix it yourself. Typically that means removing a dangling link, or uninstalling the other copy with the tool that installed it. Run the plan again afterwards. The rest of a run proceeds without the conflicting package and its dependents, and `apply` exits 1.

## Third-party taps and trust (Orca)

Orca lives in the `stablyai/orca` tap. Homebrew 6 and later require trust for third-party taps, so the plan shows three separate steps, all covered by your one consent:

```
 7. orca  brew tap stablyai/orca
 8. orca  brew trust --cask stablyai/orca/orca   [after 7]
 9. orca  brew install --cask stablyai/orca/orca   [may ask for sudo; after 8]
```

Trust is granted for the item only, never for the whole tap. `facman doctor` reports an installed tap item that is not trusted (exit 2), with the exact `brew trust` command.

## Use your own catalog

Point any command at a catalog directory (`catalog.yaml` plus `profiles/*.yaml`):

```sh
facman plan  --catalog ./my-catalog --profile team-tools
facman scan  --catalog ./my-catalog --json
facman       --catalog ./my-catalog --profile team-tools     # TUI
```

A mistake is exit 3, with every problem listed as `file:line:col`. The report records the catalog as `dir:<path>@<sha256>`, so you can tell exactly which files were used. A minimal working catalog is [testdata/catalogs/integration/](../testdata/catalogs/integration/). Git URLs are not accepted yet (exit 64).

## Linux build machines and WSL

```sh
facman scan                                  # Homebrew at /home/linuxbrew/.linuxbrew is found even off PATH
facman plan --profile base-dev               # casks are offered only with a display
```

- Missing distro prerequisites for Homebrew are reported with the exact `apt` or `dnf` command. `apply` stops with exit 4 until you install them.
- On WSL 2, Windows GUI apps show as `unavailable-here` with the `winget install …` command to run on Windows.
- Unattended CI provisioning needs `-cl` (Increment 3). Today, use `scan --json` and `plan --dry-run` in CI to check a machine without changing it.

## Recover from an interrupted run

If a run is stopped by a reboot, a closed terminal or a crash, run the same command again:

```sh
facman apply --profile agentic-factory
```

At startup, `facman` marks the interrupted operation in the audit log. It records the package as `managed` if the install actually finished, or proposes it again if it did not. `Ctrl+C` during a run makes `facman` stop after the current command and exit 1; it says which command that is. `facman` never kills a running command. A `brew install` runs on its own and does not see the `Ctrl+C`; a `sudo` step shares your terminal, so it receives the `Ctrl+C` itself and may stop. A second `Ctrl+C` does nothing yet: to stop a hung command, end it from another terminal (see the PID with `ps`).

## Look at the audit trail

```sh
facman status
ls "$HOME/Library/Application Support/facman/logs"     # macOS
ls "${XDG_STATE_HOME:-$HOME/.local/state}/facman/logs" # Linux, WSL
sqlite3 "$HOME/Library/Application Support/facman/state.db" \
  'select id, package_id, status, exit_code, consent_by from operations order by id desc limit 10;'
```

Every action has an `operations` row with its exact commands, who consented and when, the exit code and the log path. Rows are never deleted. Log files are pruned after 90 days.

## When `facman` says no

| You see | Exit | Do this |
|---|---|---|
| `facman is running as root` | 4 | Run as your normal user, without `sudo`. |
| `another facman (PID n) is changing this machine` | 4 | Wait for it to finish, or check that the process exists. |
| `… and stdin is not a terminal` | 64 | Run in a terminal, use the TUI, or add `--dry-run`. |
| `arrives in a later increment` | 64 | The command or flag is designed but not built yet. |
| `state database … was written by a newer facman` | 4 | Use the newer binary. `facman` never migrates down. |

Back to [user-docs](README.md).
