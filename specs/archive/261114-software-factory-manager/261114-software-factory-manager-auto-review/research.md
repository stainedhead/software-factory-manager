# Research: software-factory-manager auto-review fixes

**Date:** 2026-11-14
**Source PRD:** [software-factory-manager-auto-review-PRD.md](software-factory-manager-auto-review-PRD.md)

## Research Questions

- **RQ-1 (FR-001a, OQ-4).** Do real Homebrew formula, cask and tap names fit the PRD's allow-lists? Formula and cask tokens are lowercase with `-`, `.`, `+` and `@` (for example `python@3.13`, `c++filt` style names); tap repos are `user/repo` with the `homebrew-` prefix dropped. Fully qualified names are `user/repo/name`. The embedded catalog is the check.
- **RQ-2 (FR-001b).** Which brew options does a plan legitimately need? Only `--cask` and `--formula` (on `install` and `trust`). Everything else that starts with `-` is an option that changes the reviewed command.
- **RQ-3 (FR-002).** What does `os/exec.LookPath` do with relative entries since Go 1.19? It returns `exec.ErrDot`. Our own lookup must match by skipping non-absolute entries (including the empty entry, which means the current directory).
- **RQ-4 (FR-003).** Is there a trustworthy source of real `brew info --json=v2` output without running Homebrew here? The formulae.brew.sh JSON API serves the same per-formula and per-cask objects that `brew info --json=v2` wraps in `{"formulae":[…],"casks":[…]}`. Running the real installer is not safe in this container or on hosted CI without a disposable VM.
- **RQ-5 (FR-007b).** How is a signal death reported by `os/exec`? `ProcessState.ExitCode()` is -1 and `Sys().(syscall.WaitStatus).Signal()` gives the signal; the shell convention is `128+signo`.

## Industry Standards

- Shell convention: exit status `128+n` for death by signal `n`.
- Go's `exec.ErrDot` (Go 1.19) for relative PATH lookups.

## Existing Implementations

See the Increment 1 code under `internal/`; findings and line references are in the PRD.

## API Documentation

- Homebrew JSON API: `https://formulae.brew.sh/api/formula/<name>.json`, `https://formulae.brew.sh/api/cask/<token>.json`.

## Best Practices

- Validate data at the boundary (catalog load), and keep the domain invariant as a second line of defence (plan validation).
- Escape, don't strip, control characters in consent text so the user sees that something odd is there.

## Open Questions

PRD OQ-1 to OQ-4 keep their defaults; decisions are in [implementation-notes.md](implementation-notes.md).

## References

- PRD findings FR-001 to FR-019.
