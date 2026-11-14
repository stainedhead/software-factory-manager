# Catalog format and state database

The reference for the two formats `facman` reads and writes: the catalog and profile YAML (`apiVersion: facman/v1`), and the tables of the state database. It replaces §4 and §6 of the archived Increment 1 data dictionary as the place to look. The user-facing rules for `--catalog` directories (allow-lists, exit codes) are in [user-docs/configuration.md](../user-docs/configuration.md#catalog-directories). Back to [docs](README.md).

## Catalog directory

A catalog is a directory with one `catalog.yaml` and a `profiles/` folder with one `<name>.yaml` per profile. The embedded catalog is [internal/adapter/catalogyaml/embedded/](../internal/adapter/catalogyaml/embedded/). `--catalog <dir>` replaces it whole. The smallest working example is [testdata/catalogs/integration/](../testdata/catalogs/integration/).

Decoding is strict. An unknown key, a wrong type, or a value outside the allow-lists is exit 3 with `file:line:col` and the field path. The two exceptions are `service:` and `policy:`, which are kept opaque for later increments. Order is preserved from the YAML node order.

## `catalog.yaml`

```yaml
apiVersion: facman/v1
packages:
  <package id>:          # lowercase letters, digits and . _ + @ -, starting with a letter or digit
    display: <text>      # name shown in every screen
    category: <text>
    kind: cli | gui      # required, except on a choice group
    platforms: [darwin, linux, wsl]   # optional: where the entry is offered at all
    detect: { ... }      # how scan finds it (below)
    install: { ... }     # per platform, ordered steps (below)
    version_policy: track | pinned | delegate   # default track
    requires: [<id>, {package: <id>, min_version: "x.y"}]
    satisfies_if: { min_version: "24.11" }      # any install at or above counts as present
    reject_if: { version_lt: "2.0" }            # a found install below this is not the tool
    choice: [{id: <member>, platforms: [...], host_on: [wsl]}]   # a choice group (one of)
    satisfied_by_any_present: true              # a choice group is met by any member present
    sign_in_required: true                      # the Summary prints a sign-in step
    superseded_by: <id>                         # detection-only entry: never installed, reported foreign
    service: { ... }                            # opaque until Increment 2
```

### `detect`

| Key | Meaning |
|---|---|
| `commands` | Bare executable names looked up on the absolute entries of `PATH` (no `/`, no `..`, no leading `-` or `.`) |
| `version` | `{cmd: [<one of commands>, args...], regex: '<RE2 with one group>'}`: the version probe. `cmd[0]` must be one of `commands` |
| `provenance` | A map of provenance to path glob, for example `brew-cask: "{brew_prefix}/Caskroom/claude-code*/**"`. Templates: `{brew_prefix}`, `{npm_prefix}`, `~`. Provenances: `brew`, `brew-cask`, `native`, `npm`, `nvm`, `fnm`, `asdf`, `volta`, `script`, `app-bundle`, `os` (and `unknown` when nothing matches) |
| `bundle_ids` | macOS app bundle IDs, found with Spotlight and `/Applications` |
| `npm_package` | The global npm package name |
| `winget_id` | The Windows-host package, for the WSL hint |

### `install`

`install` maps a platform key (`darwin`, `linux`, `wsl`) to an ordered list of steps. The first step whose backend this build implements is used; a `report` or `host-report` step makes the entry `unavailable-here` with its hint. A missing platform means the entry is not offered there (`unavailable-here`). `wsl` inherits `linux` for `cli` entries.

| Key | Meaning |
|---|---|
| `backend` | `brew-formula`, `brew-cask`, `npm` (accepted, planned from Increment 2), `report` or `host-report` (never install; show `hint`) |
| `id` | The formula, cask or npm package: `name` or `user/repo/name` for brew, never starting with `-` |
| `tap` | `user/repo`. A step with a tap must name an item of that tap (`stablyai/orca/orca` for `stablyai/orca`) |
| `trust` | `item`: grant tap trust for this one item only (`brew trust --cask <item>`). Whole-tap trust does not exist |
| `version` | A pin, used with `version_policy: pinned` |
| `hint` | Text shown for `report`/`host-report` steps |
| `post_install` | Lines the Summary shows after the install |
| `link` | Rejected: `facman` never runs `brew link --force`; keg-only formulae go on `PATH` instead |

No value may contain a control character (escape, newline, tab and the like).

## `profiles/<name>.yaml`

```yaml
apiVersion: facman/v1
profile: agentic-factory          # same rules as a package id
description: <text>
include: [base-dev]               # other profiles, unioned
packages:
  - jq                            # required
  - t3-code: { optional: true }   # off unless --with t3-code (or ticked in the TUI)
  - node: { version: "24" }
policy: { ... }                   # opaque in Increment 1
```

`--profile` can repeat; the profiles are unioned. An unknown package ID, a cycle in `include` or `requires`, or conflicting pins is exit 3.

## State database

`<state dir>/state.db` is SQLite in WAL mode (pure Go, `modernc.org/sqlite`), readable with the system `sqlite3`. The state directory is listed in [user-docs/configuration.md](../user-docs/configuration.md#state-and-logs). Migrations are numbered and embedded in [internal/adapter/sqlite](../internal/adapter/sqlite/). Each runs in a `BEGIN IMMEDIATE` transaction, so simultaneous first launches are safe. `facman` never migrates down: a database with a newer `schema_version` is exit 4.

| Table | Columns | Notes |
|---|---|---|
| `meta` | `key`, `value` | `schema_version` (1), `machine_id` (random UUID), `last_manifest_rev` |
| `packages` | `id`, `state` (`managed` or `adopted`), `backend`, `backend_ref`, `installed_version`, `install_path`, `installed_at`, `installed_by`, `adopted_at`, `manifest_rev`, `scope` (`user`), `bootstrap` | Only what `facman` owns. Other states (`absent`, `foreign`, `drifted`, `duplicate`, `conflict`, `unavailable-here`) are computed per scan and never stored. The Homebrew bootstrap writes `id = 'homebrew'`, `backend = 'script'`, `bootstrap = 1` |
| `operations` | `id`, `started_at`, `finished_at`, `package_id`, `op`, `commands_json`, `exit_code`, `status`, `log_path`, `consent_by` | Append-only audit log, never pruned. A row is written `in_progress` before its command runs. A signal-killed command records `128 + signo`. An interrupted row is marked at the next launch |
| `detections` | `scanned_at`, `package_id`, `found`, `version`, `provenance`, `path` | The last 10 scans per package |
| `tap_trust` | `package_id`, `tap`, `item`, `granted_at`, `operation_id` | One row per item-trust grant |
| `services` | `package_id`, `manager`, `unit_path`, `port`, `exposure`, `last_health`, `last_backup_path` | Created empty; Increment 2 writes it |

Times are RFC 3339 UTC text. `consent_by` is `<os user>@<RFC 3339 time>`.
