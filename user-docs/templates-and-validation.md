# Templates and validation

Every file you write for `facman` can be generated from a template and checked before you commit it.

## Templates

| Command | Creates |
|---|---|
| `facman template config-list [--profile P]` | A configuration list ([guide](configuration-list.md)) |
| `facman template hermes` | A Hermes setup spec + `assets/` ([guide](hermes-setup.md)) |
| `facman template paperclip` | A Paperclip setup spec + `company/` skeleton ([guide](paperclip-setup.md)) |
| `facman template orca` | An Orca setup spec ([guide](orca-setup.md)) |
| `facman template t3` | A T3 setup spec + `settings.fragment.json` ([guide](t3-setup.md)) |

Flags: `-o <file>` (output path), `--minimal` (no comments), `--from-machine` (pre-fill from this machine; secret values are never copied).

## Validation

```bash
facman validate <file|dir>…              # kind detected automatically
facman validate setup/ --deep            # also run each tool's own checker, if it's installed
facman validate configs/ setup/ --strict # treat version-fit warnings as errors
facman validate configs/ setup/ --format github   # annotations on the PR
```

What it checks:

1. **Syntax**: YAML and JSON errors, with `file:line:col`.
2. **Schema**: wrong keys, types and values, with the line and the allowed values.
3. **Meaning**: unknown package IDs, missing choice picks, bad option values, missing inputs for remote exposure, missing asset files, undeclared env var names.
4. **Secrets**: anything that looks like an API key or token is an **error**.
5. **Version fit**: the file's `tool_version` range against the version the catalog pins.
6. **`--deep`**: Paperclip `company import --dry-run`, `hermes config check` (sandboxed), T3 key check.

Example output:

```
setup/hermes/team.setup.yaml:14:5  error   configure[1].type: "file" is not one of files, env_file, command, json_merge
configs/build-linux.facman.yaml:22:7  error   packages[9]: choice group "container-runtime" needs choice: orbstack | colima | docker-desktop
setup/t3/providers.setup.yaml:5:15 warning tool_version ">=0.1.9 <0.2" but catalog pins t3-code 0.2.1
3 files, 2 errors, 1 warning
```

`facman apply` and `facman setup` always validate first and stop on errors.

## Editor support (VS Code)

Templates start with a schema header:

```yaml
# yaml-language-server: $schema=https://<schemas-host>/facman/tool-setup.v1.json
```

With the **YAML** extension (Red Hat) installed, you get autocomplete, hover docs and red squiggles as you type. To map schemas for a whole repo:

```bash
facman schema export --vscode     # writes .vscode/settings.json yaml.schemas entries
```

## In the TUI

The **Files** tab lists every configuration and setup file in the current repo with its validation status. Keys:

- `t`: new file from a template
- `e`: open in `$EDITOR` (revalidated on save)
- `v`: validate
- `d`: diff against what is applied

## In CI

```yaml
# .github/workflows/validate.yml
on: [pull_request]
jobs:
  validate:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: stable }
      - run: go run github.com/stainedhead/software-factory-manager/cmd/facman@v0.3.0 validate configs/ setup/ --strict --format github
```

## Why formats stay correct

Each `facman` release and each catalog bump runs **contract tests**. These install the pinned tool versions, apply known-good example files, and run each tool's own checker. If a vendor changes a format (T3 and Hermes are the likeliest), the build fails before you do. See the design doc, §7b.
