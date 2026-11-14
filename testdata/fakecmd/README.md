# testdata/fakecmd

`main.go` is a fake multi-call program. Linked under a program name, it appends the call to the log, answers from the first matching rule in the world file next to it (or `$FAKECMD_DIR`), and otherwise runs the emulated Homebrew from [internal/testkit/fakeworld](../../internal/testkit/fakeworld/). It locks the directory with `flock` so parallel detectors cannot corrupt the world.

Built only by [internal/testkit](../../internal/testkit/) (`go build ./testdata/fakecmd`); it is never part of the product. Back to [testdata](../README.md).
