# Configuration lists (`--configuration-list` / `-cl`)

> **Status: planned for Increment 3.** This guide describes a designed feature that does not ship yet: `-cl`/`--configuration-list` (and `--set`, `--check`) exit 64 until then, with a message naming Increment 3. See [user-docs](README.md) for what ships today.

A **configuration list** is a YAML file that records every answer `facman` would otherwise ask for: which tools, which versions, which options, and how to treat tools already on the machine. Applying it asks no questions, which makes it the way to set up **build machines and CI runners**, or to make every developer machine in a factory identical.

## Build one

**Option A: from the TUI (easiest).** Run `facman`, pick your tools and options on the Plan screen, then press **`s`** and name the file. Nothing is installed.

**Option B: from a template.**

```bash
facman template config-list --profile agentic-factory -o configs/build-linux.facman.yaml
```

**Option C: copy a known-good machine.**

```bash
facman config export -o configs/clone-of-lab-07.facman.yaml   # what this machine currently manages
```

## File reference

```yaml
# yaml-language-server: $schema=https://<schemas-host>/facman/configuration-list.v1.json
apiVersion: facman/v1
kind: ConfigurationList
metadata:
  name: build-linux                 # required, unique within the repo
  description: CI runners for the agentic factory
catalog:
  source: git+https://github.com/<org>/facman-catalog.git
  ref: v2026.11.1                   # required: pin a tag, never a branch
platforms: [linux]                  # darwin | linux | wsl; refuses to run elsewhere
packages:                           # required: what this machine gets
  - id: git
  - id: gh
  - id: node                        # node@24; needed by Paperclip
  - id: awscli
  - id: claude-code
    version: "2.1.311"              # optional pin
  - id: codex
  - id: uv
  - id: jq
  - id: ripgrep
  - id: container-runtime
    choice: colima                  # choice groups MUST name a pick
  - id: paperclip
    options:
      exposure: local               # local (default) | remote
    setup: ../setup/paperclip/factory.setup.yaml   # optional: same as -ps
  - id: hermes-agent
    setup: ../setup/hermes/team.setup.yaml          # optional: same as -hs
policy:
  foreign: skip                     # skip | adopt | fail: tools that were already installed
  unavailable_here: warn            # warn | fail: e.g. GUI apps on a headless runner
  allow_sudo: false                 # package installs that need sudo fail fast
  prune: false                      # true: uninstall managed tools no longer listed
  config_drift: keep                # keep | overwrite | fail: setup files a user edited
  format_mismatch: fail             # fail | warn: installed tool newer than the schema
  conflicts: fail                   # fail | move_aside: stray files where a package installs
  tap_trust:                        # required for packages from third-party taps
    - cask:stablyai/orca/orca        # item-level only; whole taps need allow_whole_tap_trust
bootstrap:
  homebrew: auto                    # auto | never
  node: auto
  allow_sudo: true                  # first Homebrew install needs `sudo -n`
  persist_shellenv: true            # add brew to the login profile
```

### Rules to know

- **Risky options must be written out.** Paperclip `exposure: remote` only happens if the file says so, and it also needs `public_base_url` and `allowed_hostnames`:
  ```yaml
  options:
    exposure: remote
    public_base_url: https://paperclip.lab.example.com
    allowed_hostnames: [paperclip.lab.example.com]
  ```
- **No secrets.** List secret *names* in setup files (`requires_env`). Provide the values at run time from CI secrets or your shell.
- **Optional tools** (container runtime, gcloud, az, Session Manager plugin) are only installed if they're listed.

## Validate, preview, apply

```bash
facman validate configs/build-linux.facman.yaml
facman apply -cl configs/build-linux.facman.yaml --dry-run      # see exactly what would happen
facman apply -cl configs/build-linux.facman.yaml                # no prompts
facman apply -cl configs/build-linux.facman.yaml --check        # CI gate: exit 2 if the machine differs
```

Override one tool's setup for a single run:

```bash
facman apply -cl configs/build-linux.facman.yaml -ps setup/paperclip/ci.setup.yaml
```

## GitHub Actions example

```yaml
jobs:
  build:
    runs-on: ubuntu-24.04            # has Homebrew (off PATH) and passwordless sudo
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 24 }   # Paperclip needs >= 24.11; the runner default is 22
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: go run github.com/stainedhead/software-factory-manager/cmd/facman@v0.3.0 apply -cl configs/build-linux.facman.yaml --json
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success, or already compliant |
| 1 | An install, upgrade or setup step failed |
| 2 | Drift found (`--check`) |
| 3 | Validation error (bad file, missing input, missing secret, tool not installed for a setup) |
| 4 | Bootstrap failed (no Homebrew and no `sudo -n`, running as root, unknown distro) |
| 64 | Usage error (e.g. `-cl` with no filename) |

## Troubleshooting

- **"It seems there is already a Binary at /opt/homebrew/bin/…".** A file that Homebrew doesn't own (often a leftover npm global launcher) is in the way. Run `facman doctor` to see what it is. Then either remove it through its owner (`npm uninstall -g …`), or set `policy.conflicts: move_aside` so `facman` moves it to its backups folder before installing.
- **"needs tap trust".** Add the item to `policy.tap_trust`, for example `cask:stablyai/orca/orca`.

- **"Homebrew refuses to run as root".** Container jobs often run as root. Use a non-root user with passwordless sudo, or the `homebrew/brew` image.
- **"choice group container-runtime needs a pick".** Add `choice: colima` (or `orbstack` / `docker-desktop`).
- **GUI apps skipped on CI.** This is expected: headless runners report them as `unavailable-here`.
