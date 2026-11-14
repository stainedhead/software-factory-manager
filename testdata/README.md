# testdata

Test fixtures and test-only programs. Go ignores `testdata/` for `./...`, so nothing here is built into the product.

| Path | Contents |
|---|---|
| [fakecmd/](fakecmd/) | Fake multi-call executable (brew, xcode-select, mdfind, plutil, npm, getconf, ldd, file), built by `internal/testkit` |

Machine fixtures (`fixtures/`) and the one-entry integration catalog (`catalogs/integration/`) are added by later workstreams. Back to the [root README](../README.md).
