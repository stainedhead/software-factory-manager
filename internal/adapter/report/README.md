# internal/adapter/report

Report schema v1, the `--json` output of `scan`, `plan`, `apply`, `status` and `doctor` (FR-007, data-dictionary §7, AD-11).

| File | Contents |
|---|---|
| `schema/report.v1.json` | JSON Schema 2020-12, embedded as `report.Schema`; `additionalProperties: false` everywhere |
| `report.go` | `Input` (bootstrap report, catalog, desired set, statuses, plan, execute outcome, status, doctor, summary, error), `Build(Input) Document`, `Encode(w, Document)` |
| `report_test.go` | Every command's document validates against the embedded schema (santhosh-tekuri/jsonschema, tests only); broken documents fail; optional fields omitted; `consent_by` null on dry run and decline |

Rules: optional fields are omitted, never `null`, except `consent_by` on `apply` (null when nothing was consented). `exit_code` is the process exit code. Fields beyond data-dictionary §7 (all optional): `error`, `tap_trust` (status), `packages[].record`, `offered`, `satisfying`, `detail`, `results[].detail`, `plan.actions[].tap/item` (implementation-notes I-33). Doctor findings carry `package_id` when the message names a catalog package.

**Architecture.** Adapter; imports domain types and use case result types (`execute`, `status`, `doctor`, `summary`). Used by [cli](../cli/). Back to [internal/adapter](../README.md).
