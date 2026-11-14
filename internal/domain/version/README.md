# internal/domain/version

Parses and compares the version strings tools print (`2.1.4`, `v24.11.0`, `1.7.1-rc1`). Unknown or unparsable versions never satisfy a minimum.

| File | Contents |
|---|---|
| `version.go` | `Version`, `Parse`, `Compare`, `AtLeast`, `Satisfies` |

**Architecture.** Domain layer: no I/O. Imports nothing. Used by `platform` (glibc), `ownership` (`satisfies_if`, `reject_if`).

Related: [data dictionary](../../../specs/261114-software-factory-manager/data-dictionary.md), [architecture](../../../specs/261114-software-factory-manager/architecture.md). Back to [internal/domain](../README.md).
