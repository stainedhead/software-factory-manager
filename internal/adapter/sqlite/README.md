# internal/adapter/sqlite

The inventory store (FR-060 to FR-062, AD-5): pure-Go SQLite (`modernc.org/sqlite`, `CGO_ENABLED=0`) implementing the `Inventory` ports of `usecase/scan`, `usecase/execute` and `usecase/status`.

| File | Contents |
|---|---|
| `store.go` | `Open(ctx, path, now)`, numbered migrations, `meta` (`schema_version`, `machine_id`, `last_manifest_rev`), `Meta`/`SetMeta`, `Close` |
| `inventory.go` | `Records`, `UpsertRecord`, `StartOperation`, `FinishOperation`, `Operations`, `MarkInterrupted`, `SaveDetections`, `Detections`, `RecordTapTrust`, `TapTrustGrants` |

- Migration 1 creates `meta`, `packages`, `operations`, `detections`, `services` (empty until Increment 2) and `tap_trust` exactly as in [data-dictionary §4](../../../specs/archive/261114-software-factory-manager/data-dictionary.md#4-persisted-schema-sqlite-migration-1). Opening is WAL, `busy_timeout(5000)`, `foreign_keys(1)`, one connection.
- A file that is not SQLite, or a `schema_version` above `SchemaVersion`, is a bootstrap error (exit 4) naming the path; the file is never modified or deleted, and facman never migrates down.
- Times are RFC 3339 UTC text (second precision); `commands_json` holds `[{argv, env_names, needs_sudo, unverified, interactive, mutating}]`. The file is readable with the system `sqlite3`.
- `FinishOperation` only moves `in_progress` rows and stamps `finished_at` from the clock given to `Open` (I-11). `MarkInterrupted` moves every `in_progress` row to `interrupted` and returns them; call it only while holding the state lock. `SaveDetections` keeps the last 10 scans per package; `operations` is never pruned.

**Architecture.** Adapter; imports domain `inventory`, `plan`, `catalog`, `failure`, `appinfo`. Back to [internal/adapter](../README.md).
