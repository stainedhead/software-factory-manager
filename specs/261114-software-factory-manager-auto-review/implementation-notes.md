# Implementation notes: software-factory-manager auto-review fixes

**Date:** 2026-11-14

Purpose: decisions, per-fix reviews, edge cases and deferrals made while implementing the review fixes. Update this file with every non-obvious decision and after each fix's self-review.

## Technical Decisions

- **RF-0 (process).** One sequential worker, no teammates or worktrees: the run is unattended and single-agent, and sequential order removes the PRD's merge-order risk between streams A and D.

- **RF-1 (FR-001a, detail format).** The PRD asks for a `file:line:col`-style detail. Positions exist only in the YAML decoder, while the allow-lists belong in the domain (`catalog.Validate`), so details use the existing field-path style (`packages.evil: install.darwin[0]: brew id "-f" is not allowed`). This is the same style every other `Validate` problem already uses, and it names the exact field.
- **RF-2 (FR-001a, e2e "no brew invocation").** The bootstrap check (`brew --version`, `brew shellenv`) runs before the catalog loads, because the exit-code precedence 4 > 3 (I1-12) needs it first. These probes do not depend on the catalog. The e2e test asserts that no other brew command runs (no `info`, `tap`, `trust` or `install`).
- **RF-3 (FR-001a, beyond the PRD).** Added to the allow-lists: (a) `detect.version.cmd[0]` must be one of `detect.commands`, because `probeVersion` replaced the program by the resolved path only when it matched, so any other program from data (`rm`) would run during `scan`; (b) a brew step with a `tap` must name an item of that tap, so trust and install always target the same item; (c) npm IDs get an npm-name pattern; (d) no control character in any text a catalog can show (display, category, hints, versions, globs, bundle and winget IDs, profile descriptions). The embedded catalog passes all of them.
- **RF-4 (FR-001b).** Homebrew's option parser accepts abbreviated long options (`--forc`), so the brew rule is an exact allow-list: after the subcommand only `--cask`/`--formula`, on `install` and `trust`. `tap` takes no option. For every program, `forceLike` also rejects any long option that starts with or abbreviates `force`/`overwrite` (case-insensitive, before `=`) and any short cluster containing `f` (`-f`, `-fsSL`). `testkit.UnsafeReasons`, the independent check on every e2e invocation log, gained the same short-force and brew-option rules, written separately so it stays an independent oracle.
- **RF-5 (FR-001c).** Quote when any rune is not `strconv.IsPrint` or the string is not valid UTF-8. That covers C0 and C1 controls (`unicode.IsControl`) and also format characters such as U+202E (bidi override), which can reorder text on screen.

- **RF-6 (FR-002, scope).** The finding also applied to `brew.Locator.Locate`, which walked `PATH` with its own loop and located `./brew` (reported as Homebrew at prefix `.`). It now skips non-absolute entries too. Go's `os/exec` refused to start the relative `brew`, which is why nothing ran before the fix, but the located brew and the detections (`node` at `node`) were still wrong. The e2e test asserts both, and was confirmed red without the fix.
- **RF-7 (FR-002, finding).** `path-relative-entry` is computed in `usecase/scan` from `Env.Path` (the child PATH, which contains the base PATH), lists the entries Go-quoted (so an entry with a control character cannot reach the terminal), and is a warning, not fatal: facman stays safe by ignoring the entries.

- **RF-8 (FR-003, partial).** Done here: (a) `infoShapeProblems`, a type check of every `brew info --json=v2` field facman reads, run on the fixtures by `TestInfoShape` (so the hand-written files are at least consistent with the parser's expectations); (b) `TestIntegrationBrewContract` (`-tags integration`, read-only), which runs `brew info --json=v2` for `--installed`, `hello`, `sqlite` (keg-only, macOS) and the `zoom` cask (macOS), plus `brew tap` and `brew trust`, checks the shapes, logs the raw output in the CI log, and writes it to `$FACMAN_CAPTURE_DIR` when set; (c) the clean-machine procedure and the capture procedure in `user-docs/field-check.md`. `brew trust` without arguments skips rather than fails when it errors, because its listing format is still open (R-OQ-1) and a red CI for an open question helps nobody.
- **RF-9 (FR-003, deferred parts).** Not done: AC 1 (a CI container job that runs the real Homebrew installer) and replacing the fixtures with captured files. The installer cannot be run safely in this container (root, a real machine), and the dev-flow brief rules out real installs on hosted CI in this run. No trustworthy source of real output was reachable: the Homebrew JSON API (formulae.brew.sh) is refused by this session's egress proxy and the Homebrew GitHub repository is not enabled for this session. The fixtures stay hand-written and are marked so. Follow-up for J: run part 2 and part 3 of `user-docs/field-check.md`, then record the run below. The `zoom` cask is used as the pkg-artifact example on the belief that it ships a `pkg`; the test only logs `NeedsSudo` for it, so a wrong belief cannot turn CI red.

### macOS clean-machine run

| Field | Value |
|---|---|
| date | not run (no clean macOS VM in this unattended cloud run) |
| macOS version | — |
| Homebrew version | — |
| `facman apply` exit code | — |
| `packages` rows (`homebrew`, one cask) | — |

The run is step 2 of [user-docs/field-check.md](../../user-docs/field-check.md#part-2-clean-machine-bootstrap-review-fr-003), handed to J next to I1-16.

## Per-fix reviews

- **FR-001a.** Correct: every value that reaches argv (`id`, `tap`), a path (`id` in log names, `detect.commands` joined onto directories) or the terminal is checked. Error details quote values with `%q`, and `safeLabel` quotes a key with a control character, so the error itself cannot carry an escape to the terminal. Architecture: domain-only (`regexp`, `unicode` from the standard library). No path can pass `-f`/`--force` through an ID (an ID cannot start with `-`), and a tap cannot start with `-`. Whole-tap trust is still rejected by `trust: item` and the tap-item rule.
- **FR-001b.** Correct for every plan command: the installer argv (`/bin/bash <file>`) has no options, brew commands carry only `--cask`/`--formula`. Option-before-subcommand (`brew -f install`) is caught as a forbidden subcommand. Whole-tap trust check unchanged. Domain-only.
- **FR-002.** Every PATH walk in the code (`execrunner.lookPath`, `detect.PathDetector`, `brew.Locate`; found by grepping `SplitList` and `"PATH"`) now uses `filepath.IsAbs`. A name with a slash is still checked as given, which is safe because catalog commands cannot contain `/` (FR-001a) and plan argv[0] is `brew` or `/bin/bash`. Architecture: `usecase/scan` uses only the standard library and `domain/platform` for the new code.
- **FR-003 (partial).** The contract test runs only read-only brew commands with `HOMEBREW_NO_AUTO_UPDATE=1`, and skips as root or without Homebrew, so it is safe on any machine. The shape checker lives in an untagged test file so it is covered by `make test`. No production code changed.
- **FR-001c.** Both renderers (TUI Review and headless `plan`) and the progress lines use `Command.String`; a grep found no other place that joins argv for display. The TUI test checks the unstripped render has no raw ESC from the command, and was confirmed red against the old `String`.

## Edge Cases & Solutions

## Deviations from Plan

## Deferred items

## Lessons Learned
