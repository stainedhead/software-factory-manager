# internal/domain

Innermost layer: entities, value objects and domain errors. No I/O, and nothing from other layers of this repo. Domain packages may import each other in this order only: `version`, `failure` → `platform` → `catalog` → `plan` → `inventory` → `ownership`.

| Package | Purpose |
|---|---|
| [appinfo/](appinfo/) | Application identity (name, full title, description) and the `--version` / `--about` text |
| [version/](version/) | Version parse and compare (unknown never satisfies) |
| [failure/](failure/) | Typed errors and exit codes (64, 3, 4, 1, 2) |
| [platform/](platform/) | Platform facts, `BootstrapReport`, findings and which are fatal per command |
| [catalog/](catalog/) | Catalog and profile model, validation, install selection, profile resolution |
| [plan/](plan/) | Commands, actions, plans, conflicts, consent; ordering and safety invariants |
| [inventory/](inventory/) | Stored records, operations log and status transitions, snapshots, tap-trust grants |
| [ownership/](ownership/) | Detections and the classification into one ownership state per package |

Used by [../usecase/](../usecase/) and [../adapter/](../adapter/). Back to [internal](../README.md).
