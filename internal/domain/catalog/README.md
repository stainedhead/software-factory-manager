# internal/domain/catalog

The catalog model, validation, per-platform install selection and profile resolution.

| File | Contents |
|---|---|
| `catalog.go` | `Entry` (with `SupersededBy` for detection-only entries such as `gemini-cli`), `Catalog`, `Profile`, `DetectSpec`, `InstallStep`, `Kind`, `BackendID`, `Provenance`, `VersionPolicy` |
| `validate.go` | `Catalog.Validate`: every problem in one `failure.Validation` (duplicates, unknown requires/choice members/profile packages/includes, missing kind, unknown backend, `trust` other than `item`, requires and include cycles; a superseded entry must name a known replacement, have no install block and be in no profile) |
| `values.go` | The value allow-lists (review FR-001a): package IDs and profile names, brew and npm IDs, taps (and tap items inside their tap), bare detect commands, the version probe program, and no control characters in any text |
| `install.go` | `SelectInstall(entry, platform, implemented)` → `Availability` (offered / available / unavailable-here with reason and hint; WSL inheritance and host items; no-display rule) |
| `provenance.go` | `Locations` (`{brew_prefix}`, `{npm_prefix}`, `~`), `MatchGlob` (`**`, `*`, `?`), `ProvenanceOf(entry, path, locations)`: the entry's provenance globs first, then the generic rules (Caskroom, Cellar/opt, npm global modules, nvm/fnm/asdf/volta, OS directories); used by the detectors and `adapter/fsprobe` |
| `resolve.go` | `Resolve(catalog, profiles, with)` and `ResolveSelection(catalog, Selection{Profiles, With, Without})` → `DesiredSet` (includes, union, required beats optional, pins, `--with`, choice picks, deselections from the TUI Plan screen, transitive requires) |

**Architecture.** Domain layer: no I/O. Imports `platform`, `failure`. Used by `plan`, `ownership`, the use cases and `adapter/catalogyaml` (which decodes YAML into these types).

Related: [data dictionary](../../../specs/archive/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/archive/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
