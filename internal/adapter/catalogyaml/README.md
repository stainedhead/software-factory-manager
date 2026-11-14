# internal/adapter/catalogyaml

Loads the catalog and profiles from YAML and implements the `CatalogSource` ports of `usecase/scan`, `usecase/planning` and `usecase/doctor`.

| File | Contents |
|---|---|
| `source.go` | `Source`, `Embedded(version)` (catalog v1 built in with `embed`, revision `embedded:<version>`), `Dir(path)` (`--catalog <dir>`: `catalog.yaml` + `profiles/*.yaml`, revision `dir:<abs path>@<sha256 of the files>`), `Load(ctx)` |
| `decode.go` | Strict YAML decoding with `go.yaml.in/yaml/v3`'s node API: unknown keys, wrong types and `link:` are errors with `file:line:col path: message`; `service:` and `policy:` are kept opaque; `packages` keeps file order (the planner's tie-break order) |
| `embedded/catalog.yaml` | Catalog v1 (PRD Appendix E, design §5): the 25 Appendix E packages plus `orbstack`, `colima`, `docker-desktop`, `docker`, `docker-compose`, and the detection-only `gemini-cli` (`superseded_by: antigravity-cli`, never installable) |
| `embedded/profiles/` | `agentic-factory.yaml` (includes `base-dev`; optional items off) and `base-dev.yaml` |

Errors: a git URL passed to `Dir` is a usage error (exit 64, "git catalogs arrive in a later increment"); a missing directory or file, a YAML problem, a profile whose `profile:` does not match its file name, or a `catalog.Validate` problem is a validation error (exit 3) listing every problem.

Catalog facts marked "unverified" in `catalog.yaml` (Orca, T3 Code and GitHub Copilot app bundle IDs) are data; fix them there after the I1-16 field run, no code change needed.

**Architecture.** Adapter: imports `internal/domain/catalog`, `failure`, `platform` and the port packages it satisfies. Used by `cmd/facman` wiring. Related: [data dictionary §6](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#6-catalog-and-profile-yaml-embedded-apiversion-facmanv1). Back to [internal/adapter](../README.md).
