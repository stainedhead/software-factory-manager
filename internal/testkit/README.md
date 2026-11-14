# internal/testkit

**Test-only.** Helpers imported only from `_test.go` files; nothing in the product depends on it.

| File | Contents |
|---|---|
| `testkit.go` | `FakecmdBinary` (builds [testdata/fakecmd](../../testdata/fakecmd/) once per test process), `NewFake` (temp bin dir with fakecmd linked as `brew`, `xcode-select`, `mdfind`, `plutil`, `npm`, `getconf`, `ldd`, `file`), `Fake.World`/`SetWorld`/`Log`/`Env`, `AssertSafeInvocations` and `UnsafeReasons` |
| [fakeworld/](fakeworld/) | The fake's world (rules + emulated Homebrew) and invocation log format |

`AssertSafeInvocations(t, log)` fails on `--force`/`--overwrite` anywhere, brew `upgrade`/`uninstall`/`reinstall`/`untap`/`untrust`/`link` and similar, whole-tap `brew trust <tap>`, and `HOMEBREW_NO_REQUIRE_TAP_TRUST*` in a child's environment (I1-8, I1-9, I1-10). Run it at the end of every end-to-end test.

Typical use:

```go
f := testkit.NewFake(t, fakeworld.World{Brew: &fakeworld.Brew{Prefix: t.TempDir()}})
// run code with PATH=f.Dir ...
testkit.AssertSafeInvocations(t, f.Log(t))
```

Call `testkit.RemoveBuild()` from `TestMain` to delete the built binary. Related: [plan.md testing strategy](../../specs/archive/261114-software-factory-manager/plan.md#testing-strategy). Back to [internal](../README.md).
