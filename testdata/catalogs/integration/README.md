# testdata/catalogs/integration

A one-entry catalog (GNU `hello`, profile `integration`) for the real-Homebrew integration test in [cmd/facman/integration_test.go](../../../cmd/facman/integration_test.go) (spec I1-14). The test installs `hello` through `--catalog` only when `FACMAN_INTEGRATION_MUTATE=1` on macOS, skips when `hello` is already installed, and uninstalls it afterwards. Test data only; never shipped. Back to [testdata/catalogs](../README.md).
