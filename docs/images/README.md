# docs/images

Screenshots of the `facman` TUI, used by the [root README](../../README.md) and [Getting started](../../user-docs/getting-started.md).

| File | Shows | Used by |
|---|---|---|
| `facman-plan.png` | The Plan screen for profile `agentic-factory` on an empty machine | Getting started |
| `facman-review.png` | The Review screen: the ordered `brew` commands before consent | Root README, Getting started |

## How they were made

These are not photos of a real Mac. They come from the real `facman` code running in a 100×40 pseudo-terminal through the end-to-end test fixture (`cmd/facman/harness_test.go`): a temporary `HOME`, `darwin/arm64`, and the simulated Homebrew in `testdata/fakecmd`, so nothing was installed and every package shows as absent. The terminal bytes were replayed through a terminal emulator (`pyte`) and drawn to PNG with Playwright. The fixture's temporary Homebrew prefix was replaced with `/opt/homebrew`, the Apple silicon default. The TUI draws in bold, faint and underline only, so the images have no colour beyond the window frame.

Retake them when a screen they show changes visibly: the text should match what the TUI draws today. The golden files in `internal/adapter/tui/testdata/` are the exact, tested text of each screen.

Back to [docs/](../README.md).
