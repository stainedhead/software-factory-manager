# internal/adapter/report/schema

`report.v1.json` is the JSON Schema (2020-12) for every `--json` document `facman` prints (`scan`, `plan`, `apply`, `status`, `doctor`). It is embedded in the binary as `report.Schema` and every command's output is validated against it in tests. Adding an optional field is a minor change; removing or renaming one needs schema v2. Back to [report](../README.md).
