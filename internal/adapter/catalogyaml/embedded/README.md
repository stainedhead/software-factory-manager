# internal/adapter/catalogyaml/embedded

Catalog v1 as YAML, built into the `facman` binary with `//go:embed` (only `catalog.yaml` and `profiles/*.yaml` are embedded; this README is not).

| Path | Contents |
|---|---|
| `catalog.yaml` | Every package `facman` knows: detection rules, install steps per platform, choice groups, `sign_in_required`, the detection-only `gemini-cli` |
| [profiles/](profiles/) | The built-in profiles `agentic-factory` and `base-dev` |

**Architecture.** Data for the [catalogyaml](../README.md) adapter, which decodes and validates it into `internal/domain/catalog`. The same layout is what `--catalog <dir>` accepts. Format: [data dictionary §6](../../../../specs/261114-software-factory-manager/data-dictionary.md). Back to [catalogyaml](../README.md).
