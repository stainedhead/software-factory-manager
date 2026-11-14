# internal/domain/catalog

The catalog model, validation, per-platform install selection and profile resolution.

| File | Contents |
|---|---|
| `catalog.go` | `Entry`, `Catalog`, `Profile`, `DetectSpec`, `InstallStep`, `Kind`, `BackendID`, `Provenance`, `VersionPolicy` |
| `validate.go` | `Catalog.Validate`: every problem in one `failure.Validation` (duplicates, unknown requires/choice members/profile packages/includes, missing kind, unknown backend, `trust` other than `item`, requires and include cycles) |
| `install.go` | `SelectInstall(entry, platform, implemented)` → `Availability` (offered / available / unavailable-here with reason and hint; WSL inheritance and host items; no-display rule) |
| `resolve.go` | `Resolve(catalog, profiles, with)` → `DesiredSet` (includes, union, required beats optional, pins, `--with`, choice picks, transitive requires) |

**Architecture.** Domain layer: no I/O. Imports `platform`, `failure`. Used by `plan`, `ownership`, the use cases and `adapter/catalogyaml` (which decodes YAML into these types).

Related: [data dictionary](../../../specs/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
