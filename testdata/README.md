# testdata

Test fixtures and test-only programs. Go ignores `testdata/` for `./...`, so nothing here is built into the product.

| Path | Contents |
|---|---|
| [fakecmd/](fakecmd/) | Fake multi-call executable (brew, xcode-select, mdfind, plutil, npm, getconf, ldd, file), built by `internal/testkit` |
| [catalogs/](catalogs/) | Catalog directories for tests (`e2e/`: two entries for `--catalog`, profile union, pins, crash recovery; `integration/`: GNU `hello` for the real-Homebrew test) |

The end-to-end tests build their machine fixtures in Go (`cmd/facman/e2e_test.go`) from the embedded catalog. Back to the [root README](../README.md).
