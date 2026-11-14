# Data dictionary: software-factory-manager auto-review fixes

**Date:** 2026-11-14

Purpose: the data shapes these fixes add or constrain. The Increment 1 data dictionary ([../archive/261114-software-factory-manager/data-dictionary.md](../data-dictionary.md)) stays authoritative for everything else.

## Entities

No new entities. The state DB schema is unchanged.

## Value objects (new constraints)

| Value | Where | Allowed shape |
|---|---|---|
| Package ID (catalog key) | `catalog.Entry.ID` | `^[a-z0-9][a-z0-9._+@-]*$` |
| Brew install ID | `catalog.InstallSpec.ID` (brew backends) | `^[A-Za-z0-9][A-Za-z0-9._+@-]*(/[A-Za-z0-9._+@-]+){0,2}$` |
| Tap | `catalog.InstallSpec.Tap` | `^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9._-]*$` |
| Detect command | `catalog.Entry.Detect.Commands[]` | non-empty, no `/`, no `..`, no control character |
| Log name part | `statedir.LogPath` | package ID with every byte outside `[A-Za-z0-9._+@-]` replaced by `_` |

## Interfaces

No port signature changes are planned for the P1 fixes.

## Enumerations

| Enum | New value | Meaning |
|---|---|---|
| Finding code | `path-relative-entry` | The base `PATH` has a relative or empty entry; it is ignored for lookups |

## API request/response types

Report schema v1: no new fields for the P1 fixes; the new finding code fits the existing `findings[].code` string.
