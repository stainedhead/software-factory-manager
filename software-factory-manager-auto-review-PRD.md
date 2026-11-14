# PRD: software-factory-manager auto-review fixes (Increment 1)

**Created:** 2026-11-14
**Jira:** N/A
**Status:** Draft (dev-flow `/implm-frm-prd` Step 5, `/review-code`, run unattended)
**Source review:** the whole diff from `origin/main` (`f0776c9`) to `11fed13` on branch `claude/project-thread-ys3kmd` (226 files, about 30,000 lines added), reviewed against [spec.md](specs/261114-software-factory-manager/spec.md) and acceptance criteria I1-1 to I1-16.
**Spec it fixes:** `specs/261114-software-factory-manager/` (Increment 1, Phase 0 + Phase 1 MVP)

## Executive Summary

**Verdict:** Request changes. The code is good, and nothing found blocks shipping Increment 1 to J for the I1-16 field check. Three P1 findings should land first.

Increment 1 is a well-built MVP. The layering is clean. A mechanical check (`go list -f '{{.Imports}}'` over every package) shows that `internal/domain` imports nothing outside the standard library and its own packages, and `internal/usecase` imports only `domain` and `usecase/process`. Ports are declared where they are consumed, and `cmd/facman` only wires. TDD discipline shows in the numbers: `make cover` reports 96.6% of statements, `domain` at 100% and every `usecase` package at 97% or more. The tests are table-driven and use fakes, not mocks. A shared invocation-log assertion enforces "no `--force`, `--overwrite`, `upgrade` or `uninstall`" across the e2e suite. CI is green on Linux and macOS, and the macOS run takes the mutating `hello` install (I1-14). The headline safety rules hold for the embedded catalog: commands are built as argv and never as shell strings, the installer is saved to a 0600 temp file and never piped to a shell, tap trust is granted per item, the `HOMEBREW_NO_REQUIRE_TAP_TRUST*` variables are stripped, consent is recorded, and `facman` never runs `sudo` itself (brew does, on a terminal handed over to it).

The findings cluster in three places:

1. **Values from a `--catalog` directory are not validated** before they reach argv, file paths and the terminal (FR-001). The plan invariant that is meant to forbid `--force` lets the short form `-f` through. This was reproduced: `facman plan --catalog <dir>` printed `brew install -f`.
2. **`PATH` lookups accept relative directories** (FR-002). A `.` in `PATH` makes `facman scan` execute a binary from the current directory.
3. **The riskiest path has never run for real** (FR-003). The clean-machine bootstrap (real installer, then installs) has run only against fakes, and the `brew info` fixtures were written by hand.

The rest are P2: robustness, observability, design and documentation items, including the two known ones (AGENTS.md's Commands table, and Paperclip's hint printed twice).

| Priority | Count | Findings |
|---|---|---|
| P0 (true blocker) | 0 | none |
| P1 (high) | 3 | FR-001, FR-002, FR-003 |
| P2 (medium) | 16 | FR-004 to FR-019 |

## Problem Statement

Increment 1 meets its acceptance criteria in CI, but a review of the code found gaps that the criteria do not catch. Some safety invariants can be bypassed by input from a custom catalog. Binaries can be resolved from the current directory. The clean-Mac bootstrap has never run against real Homebrew. Some failure modes leave no trace, the application logic has leaked into an adapter, and some documentation is stale or incomplete. These gaps affect the developers who will run `facman` on lab machines and the factory owners who will distribute catalogs. They are cheaper to fix now than after the I1-16 field check and Increment 2.

## Goals

- **RG1.** Every safety invariant in spec.md's Security NFR holds for any catalog that `facman` accepts, not only for the embedded one (FR-001, FR-002, FR-011, FR-012, FR-013).
- **RG2.** The clean-machine bootstrap is proven against real Homebrew, and the I1-14 evidence can be audited (FR-003, FR-004, FR-019).
- **RG3.** When a run fails or hangs in the field, the state directory has enough information to say why (FR-005 to FR-009).
- **RG4.** The layer boundaries and the documentation match AGENTS.md (FR-010, FR-014 to FR-018).

## Non-Goals

- No new user features, commands or backends. Increment 2 scope (the npm backend, `upgrade`, `uninstall`, `adopt`) stays out.
- No change to the consent model, the exit-code table (Appendix H), report schema v1 field names, or the state DB schema, except where a finding says so.
- No rewrite of `adapter/tui` rendering. Golden files change only where a finding changes visible text.
- No checksum verification of the Homebrew installer. Homebrew publishes none, and the script stays marked unverified.

## Development Guidance (mandatory for every fix)

- **TDD.** Each fix starts with a failing test that reproduces the finding (red), then the minimal change (green), then a refactor. A bug fix always adds a regression test. Use table-driven tests with `t.Run` and `t.Helper`, and fakes over mocks, as AGENTS.md requires.
- **Code review per fix.** Before starting the next fix, review the finished one (`/review-code` on its commits, or a short design and code review recorded in the fixes spec's `implementation-notes.md`). Check the Clean Architecture direction, test quality, and the acceptance criteria of that FR. Fix what the review finds before moving on.
- **Agent teammates.** Split independent workstreams across worker agent teammates (model `claude-sonnet-4-6`, per the dev-flow command). The orchestrator owns sequencing, review and `status.md`. Suggested streams: **A**, input validation and argv safety (FR-001, FR-002, FR-011, FR-012, FR-013); **B**, real-Homebrew and CI evidence (FR-003, FR-004, FR-019); **C**, state, runtime and observability (FR-005 to FR-009); **D**, design and docs (FR-010, FR-014 to FR-018).
- **Git worktrees.** Each parallel stream works in its own worktree and branch (`git worktree add ../facman-fix-<stream> -b fix/<stream>`) off the feature branch. Streams merge back one at a time, with `make check` green after each merge. Stream A touches `domain/plan` and `domain/catalog`, and stream D (FR-010) touches `adapter/session`. Land A before D's FR-010 refactor to avoid conflicts.
- **Priority order.** P1 before P2. Within a stream, follow the FR order.
- **Gate.** `PATH=$HOME/go/bin:$PATH make check` passes before every commit. Coverage stays at 80% or more overall, with new code fully covered. Docs and READMEs are updated in the same commit (AGENTS.md).

## Functional Requirements

Each finding is one FR. Line numbers are as of `11fed13`.

### P1 (high)

#### FR-001 (P1, security): Validate catalog values before they reach argv, file paths or the terminal

**Finding.** `--catalog <dir>` values flow into argv, log paths and the Review text without any charset or shape check:

- `internal/domain/catalog/validate.go:98-111` checks the backend, trust and that an `id` is present, but not what `install[].id`, `install[].tap`, the package ID (the YAML key) or `detect.commands` contain.
- `internal/domain/plan/validate.go:316-320` forbids only the long forms `--force*` and `--overwrite*`. `brew install -f` (short for `--force`) and other options such as `--HEAD` or `--ignore-dependencies` pass. Reproduced: a catalog with `id: "-f"` gives `2. evil  brew install -f   [after 1]` from `facman plan`. Only `tap: "--force-auto-update"` was caught, and only because of the long prefix.
- `internal/adapter/statedir/statedir.go:102-104` builds `logs/<ts>-<op>-<pkgID>.log` from the raw package ID. An ID with `/` or `..` writes outside `logs/`, and `OpenLog` (`:109-113`) creates the directories on the way.
- `internal/domain/plan/plan.go:70-78` (`Command.String`, used by `plan`, the Review screen and the progress output) quotes an argument only when it contains shell metacharacters. Control characters such as ESC are printed raw, so a catalog can hide part of a command on the very screen that asks for consent.
- `internal/adapter/brew/backend.go:188` and `internal/adapter/detect/path.go:111` join `detect.commands` onto directories unchecked, so `../` names probe and execute paths outside `{prefix}/bin` and PATH.

**Why it matters.** Catalogs are meant to be shared between factories (product summary), so a catalog is input. The spec's Security NFR ("no `--force`/`--overwrite`") and the consent screen both have to hold for any catalog `facman` accepts.

**Acceptance criteria.**
1. `catalog.Validate` rejects, with exit 3 and a `file:line:col`-style detail per problem: a package ID not matching `^[a-z0-9][a-z0-9._+@-]*$`; an `install[].id` for a brew backend not matching `^[A-Za-z0-9][A-Za-z0-9._+@-]*(/[A-Za-z0-9._+@-]+){0,2}$`; a `tap` not matching `^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`; and a `detect.commands` entry that is empty or contains `/`, `..` or a control character. A table-driven test covers each rule with `-f`, `--HEAD`, `../../x`, `a/b/c/d` and `"x\x1b[2K"`.
2. `plan.Validate` rejects any argument of a `brew` command that starts with `-`, except `--cask` and `--formula` for `install` and `trust`, so no short or long option can be added from data. A table-driven test covers `-f`, `--force`, `-s`, `--HEAD` and `--overwrite`.
3. `Command.String` escapes every control character (`unicode.IsControl`) whether or not the argument has shell metacharacters. A test asserts that `"a\x1bb"` renders as `"a\x1bb"` (quoted and escaped).
4. `statedir.LogPath` keeps the file inside `logs/` for any input: the name is sanitized to `[A-Za-z0-9._+@-]`, and a test asserts `filepath.Dir(LogPath(...)) == LogsPath()` for the hostile inputs above.
5. An e2e test runs `plan --catalog` on a fixture with `id: "-f"`, which exits 3 with the catalog error and records no brew invocation.

#### FR-002 (P1, security): Never resolve programs or probes from relative PATH entries

**Finding.** `execrunner.lookPath` (`internal/adapter/execrunner/runner.go:113-130`) joins each `PATH` entry with the program name and accepts the result even when the entry is relative (`.`, `bin`). This bypasses the `exec.ErrDot` protection that `os/exec.LookPath` has had since Go 1.19. The PATH detector does the same (`internal/adapter/detect/path.go:105-115`) and then runs the version probe on every match (`:121-123`). With `.` in `PATH`, `facman scan` in a cloned repository that contains an executable `./node` or `./brew` runs it. Plan commands name `brew` by bare name (FR-012), so the same lookup decides which `brew` gets the consented install.

**Acceptance criteria.**
1. `lookPath` and the PATH detector skip every non-absolute `PATH` entry. A test sets `PATH=.:<abs dir>` with an executable `brew` in the working directory and asserts that the absolute one is chosen, or `process.ErrNotFound` when there is none.
2. `scan` emits a warning finding (new code `path-relative-entry`, severity warning, with a fix telling the user to remove relative entries from PATH) when the base `PATH` has a relative entry. The report schema test covers the code.
3. An e2e test with `.` first in `PATH` and a fakecmd `node` in the working directory asserts that the fakecmd invocation log has no call from the working directory.

#### FR-003 (P1, test gap): Prove the clean-machine bootstrap and the brew parsers against real Homebrew

**Finding.** The headline flow, a clean lab machine where `facman` installs Homebrew and then the profile, has only run against fakes: `httptest` serves the installer and fakecmd plays `brew` (I1-5, `cmd/facman/e2e_test.go`). I1-14 (`cmd/facman/integration_test.go`) runs on a macOS runner that already has Homebrew, so neither the real installer, the re-locate after the bootstrap, nor `shellenv` on a fresh prefix has ever run. The `brew info --json=v2`, `brew tap` and `brew trust` fixtures were written by hand, as `internal/adapter/brew/testdata/README.md:5` says ("Provenance: not captured from a real Homebrew"), and they decide sudo hand-off, link paths and keg-only handling.

**Acceptance criteria.**
1. A new integration test (build tag `integration`, opt-in `FACMAN_INTEGRATION_BOOTSTRAP=1`) runs as a non-root user in a clean Linux container job in CI (no Homebrew at `/home/linuxbrew`). It runs `facman apply --catalog testdata/catalogs/integration --profile integration` with consent fed through a pty. It asserts that the real installer ran from a temp file, that `packages` holds `homebrew` as `managed` with `bootstrap = 1` and `hello` as `managed`, and that a second `apply` proposes nothing and exits 0.
2. The `brew` adapter fixtures are replaced by, or checked against, output captured from real Homebrew. The macOS integration job runs `brew info --json=v2` for one formula, one keg-only formula and one cask with a `pkg` artifact, plus `brew tap` and `brew trust`. A contract test parses that output with `ParseInfo`, `ParseTaps` and `ParseTrusted` and fails on unknown shapes. `testdata/README.md` records the Homebrew version the fixtures came from.
3. The macOS clean-machine path is either run once by hand on a clean macOS VM, with the result recorded in the fixes spec's `implementation-notes.md` (date, macOS and Homebrew versions, `facman apply` exit code, `packages` rows), or recorded as an explicit open item handed to J alongside I1-16.

### P2 (medium)

#### FR-004 (P2, test evidence): Make I1-14 runs auditable

**Finding.** `make integration` (`Makefile:22-23`) runs `go test` without `-v`, and the integration tests skip when Homebrew is missing or the user is root. A CI log therefore cannot tell a passed I1-14 from a skipped one. The latest macOS run only shows `ok ... cmd/facman 62.304s`.

**Acceptance criteria.**
1. `make integration` passes `-v` (or `-json` piped to a summary) so `--- PASS`/`--- SKIP` lines for `TestIntegration*` appear in the CI log.
2. When `FACMAN_INTEGRATION_MUTATE=1` is set, a missing Homebrew or a root user makes the test fail with a message instead of skipping. A unit test of the skip/fail helper covers both cases.

#### FR-005 (P2, concurrency): Make the state-DB migration atomic and run it under the lock

**Finding.** `session.start` opens and migrates the database (`internal/adapter/session/session.go:269`) before it takes the lock (`:274-283`), and read-only commands migrate without the lock at all. `sqlite.Store.migrate` reads the schema version outside the migration transaction (`internal/adapter/sqlite/store.go:118-133`) and `apply` uses a deferred `BeginTx` (`:136-160`). Two first launches at the same moment (the TUI plus `facman scan` from a script, for example) both see version 0, and the second fails with exit 4 (`table meta already exists`). Separately, `TryLock` writes the holder's PID after taking the flock (`internal/adapter/statedir/statedir.go:155-165`), so a competing process can read an empty file and report "PID 0".

**Acceptance criteria.**
1. Each migration runs in a `BEGIN IMMEDIATE` transaction that re-reads `schema_version` inside it and skips work that is already done. A test opens 20 `Store`s on one fresh file at the same time and asserts that all succeed with schema version 1 and one `machine_id`.
2. When the holder's PID cannot be read, the lock-held message says "PID unknown" and never "PID 0". A unit test covers an empty lock file.

#### FR-006 (P2, reliability): Give a hung command an escape hatch, and say truthfully what Ctrl+C does

**Finding.** Plan commands run with no timeout under `context.WithoutCancel` (`internal/usecase/execute/execute.go:326-329`). Non-interactive children get their own process group (`internal/adapter/execrunner/runner.go:191-193`), and `watchSignals` swallows every signal after the first (`internal/adapter/cli/cli.go:322-337`). A `brew install` that hangs on the network therefore can only be stopped with `kill -9` from another terminal. The message printed on the first Ctrl+C, "it is never killed midway", is also wrong for interactive (sudo) commands: they share the terminal's process group and get the SIGINT.

**Acceptance criteria.**
1. A second Ctrl+C (headless), or `y` on a second confirm in the TUI, stops the run now. It sends SIGTERM to the current child's process group, then SIGKILL after 10 s, marks the operation `interrupted` with the signal in its detail, marks the remaining actions `not-started`, and exits 1. A test drives a fakecmd that sleeps and asserts the operation row and exit code.
2. The first-signal message names the current command and, for an interactive command, says it receives the interrupt directly. A golden test covers both texts.

#### FR-007 (P2, observability): Record what happened to interactive commands and to signal-killed commands

**Finding.** Interactive commands are not teed to their per-operation log (`internal/adapter/execrunner/runner.go:188-189`), and the log only says "output went to the terminal" (`:288-290`). Every cask counts as interactive whenever `brew info` cannot be read (`internal/adapter/brew/backend.go:160-170`), which includes every cask on a machine that has no Homebrew yet, so a fresh-machine run leaves empty logs for all GUI installs. A child killed by a signal reports exit code -1 (`runner.go:199-201`), the same value as `NotRunExitCode` (`internal/usecase/execute/execute.go:21`), so the operations log cannot tell "killed" from "never started".

**Acceptance criteria.**
1. Every operation log ends with a footer giving the exit code or signal name, the start and end times and the duration, for interactive commands too. A test asserts the footer for a normal exit, a non-zero exit and a SIGTERM-killed fakecmd.
2. A signal-killed command is recorded with a detail of `killed by signal <NAME>` and an exit code distinct from `NotRunExitCode`, either a documented constant or `128+signo`, with the choice recorded in the ADR. Unit tests cover the runner and `execute`.
3. user-docs/configuration.md ("State and logs") documents that interactive output is not captured.

#### FR-008 (P2, observability): Log read-only probes so detection decisions can be explained

**Finding.** Read-only probes are not logged anywhere: `brew info`/`tap`/`trust` (`internal/adapter/brew/backend.go:80-95`), version probes (`internal/adapter/detect/path.go:166-190`), `npm prefix -g` (`internal/adapter/session/session.go:362-372`) and `file -b` (`internal/adapter/fsprobe/fsprobe.go`, `fileType`). There is no diagnostic switch. A timeout or a non-zero probe shows up only as `unknown` or an empty version. I1-16 exists to correct detection rules from lab Macs, and that is hard without a probe trace.

**Acceptance criteria.**
1. A scan that holds the lock writes `logs/<YYYYMMDD-HHMMSS>-scan.log`, which lists each probe's argv, its env variable names (never values), duration, exit code, and timeout or error. It is pruned like the other logs.
2. `scan --json` gives every `unknown` detection a `reason` that names the probe and its outcome (for example `brew info timed out after 10s`). The report schema test covers it.
3. A test with a fakecmd that times out asserts both.

#### FR-009 (P2, security hardening): Harden the installer download and record what ran

**Finding.** `internal/adapter/installer/installer.go:179` copies at most 4 MiB through `io.LimitReader` and silently accepts a truncated script. The default `http.Client` (`:112`) follows redirects to any scheme, including `http`. The bytes that run are not fingerprinted, so the operations log cannot show which installer version ran.

**Acceptance criteria.**
1. A body over the limit is an error. The file is removed and the attempt counts as failed (`httptest` test).
2. `CheckRedirect` refuses a redirect to a non-`https` URL (`httptest` test with a redirect to `http://`).
3. The SHA-256 and byte size of the downloaded file are written in the operation's log header and in `apply --json`'s action result. Tests assert both.

#### FR-010 (P2, design): Move application logic out of `adapter/session` and decouple the front ends

**Finding.** `internal/adapter/session/session.go` (630 lines) holds application rules that belong in the use-case layer. These include which items get the conflict pre-flight (`preflight`, `:457`; `brewItems`, `:473`), how a failed pre-flight turns a package `Incomplete`, and the post-bootstrap re-plan rules, present/incomplete/conflict (`Replanner.AfterBootstrap`, `:545`). They are tested only through adapter tests. The front ends depend on each other: `adapter/tui` imports `adapter/cli` (`internal/adapter/tui/tui.go:19`) for `TUIRunner`/`TUIRequest`, and `adapter/cli` depends on the concrete `session` types (`internal/adapter/cli/cli.go:26`). The use-case packages import `usecase/process` (decision I-8), but AGENTS.md still says "`usecase` imports `domain` only".

**Acceptance criteria.**
1. The pre-flight selection and the re-plan rules live in a use-case package (for example `internal/usecase/run`) behind ports, with use-case-level table tests. `adapter/session` keeps only the adapter composition, or goes away.
2. `adapter/tui` no longer imports `adapter/cli`. The shared request type lives with the consumer, and `cmd/facman` wires them.
3. AGENTS.md's Architecture section documents the `usecase/process` exception (value types only).
4. A `make archcheck` target, run by `make check` and CI, fails when `domain` imports a non-domain package of this module or a third-party package, when `usecase` imports `adapter`, `cmd` or `testkit`, or when one front-end adapter imports another. A test of the script's rule list, or its CI failure on a seeded violation, is recorded.

#### FR-011 (P2, correctness): Do not print a plan that failed its safety check

**Finding.** `planning.Build` returns the invalid plan together with the error (`internal/usecase/planning/planning.go:156-157`), `session.PlanSelection` stores it (`internal/adapter/session/session.go:439`), and `render` prints it (`internal/adapter/cli/render.go:45-48`). In the FR-001 reproduction, `facman plan` printed all four commands, then `internal error: the plan breaks a safety rule`, then exited 1. A reader sees a runnable-looking plan.

**Acceptance criteria.**
1. When `plan.Validate` fails, text output prints no command list, only the error and its details, and `--json` has an empty `plan.actions` plus the error. An e2e test with a forced invariant violation, through a fake backend, asserts both.
2. The error text says it is a bug in `facman` and asks for a report. After FR-001, catalog-caused problems are exit 3 at load time, and a test proves the FR-001 inputs never reach `plan.Validate`.

#### FR-012 (P2, correctness): Run the brew that was located and scanned

**Finding.** Detection, `brew info` and the tap checks run the located `Target.Path` (`internal/adapter/brew/backend.go:81-87`), but plan commands use the bare name `brew` (`backend.go:112-117`, `:137`, `:141`), which the runner resolves on the child `PATH` (`internal/adapter/execrunner/runner.go:160-164`). When `shellenv` cannot be read, `ChildEnv` falls back to the base `PATH` (`internal/usecase/bootstrap/bootstrap.go:225-228`). On a Mac with two Homebrews (an Intel `/usr/local` one and `/opt/homebrew`, common after a migration), the install can then go to a different Homebrew than the one scanned.

**Acceptance criteria.**
1. At run time `execute` (or the runner, through an explicit option) replaces an `argv[0]` of `brew` with the located brew's absolute path. Review keeps showing `brew …` and adds the absolute path once in its header (`Homebrew: /opt/homebrew/bin/brew`).
2. An e2e test with two fake brews on `PATH`, where the located one is not first, asserts that the install goes to the located one.

#### FR-013 (P2, security/observability): Surface inherited `HOMEBREW_*` settings that change reviewed commands

**Finding.** `Runner.Environ` passes the whole parent environment and strips only `HOMEBREW_NO_REQUIRE_TAP_TRUST*` (`internal/adapter/execrunner/runner.go:89-106`). Variables such as `HOMEBREW_CASK_OPTS` (for example `--no-quarantine` or `--appdir`), `HOMEBREW_NO_INSTALL_FROM_API` or `HOMEBREW_FORCE_BREWED_*` change what a reviewed `brew install` does. They appear neither in Review nor in the log header, which lists only override names (`:280-287`).

**Acceptance criteria.**
1. Review (TUI and headless) and every operation log header list the names of inherited `HOMEBREW_*` variables. Values are never shown.
2. `doctor` and `plan` emit a warning finding when `HOMEBREW_CASK_OPTS` contains `--no-quarantine` or `--force`, naming the variable and the fix. Unit tests cover both.

#### FR-014 (P2, TDD): Fuzz the parsers of external input

**Finding.** The parsers of untrusted or external output have only example-based tests: `brew.ParseInfo` (`internal/adapter/brew/info.go`), `brew.ParseShellEnv` (`internal/adapter/brew/locate.go:166`), `ParseTaps`/`ParseTrusted`, the catalog YAML decoder (`internal/adapter/catalogyaml/decode.go`) and the CLI short-flag normalizer (`internal/adapter/cli/normalize.go`).

**Acceptance criteria.**
1. Each of these has a native Go fuzz target (`FuzzXxx`) seeded from its testdata, and asserts no panic plus one invariant per parser (for example, `ParseShellEnv` never returns a name that fails `validName`).
2. A `make fuzz` target runs each target for 10 s. CI runs it on Linux. Any crasher found is committed as a regression seed.

#### FR-015 (P2, docs): Complete AGENTS.md's Commands table (known item)

**Finding.** The table at `AGENTS.md:28-35` lists `build`, `test`, `lint`, `fmt`, `cover` and `check`, but not `make build-all` (`Makefile:11-16`) or `make integration` (`Makefile:22-23`), which CI runs. AGENTS.md says "Use the `Makefile`; do not invent ad-hoc invocations", so contributors cannot find these targets.

**Acceptance criteria.**
1. The table has rows for `make build-all` (cross-compiles darwin/linux × amd64/arm64 into `bin/<os>-<arch>/facman`) and `make integration` (`-tags integration`, and what `FACMAN_INTEGRATION_MUTATE=1` does), plus any target added by FR-004, FR-010 or FR-014.
2. A test or script (for example in `make archcheck`) asserts that every `.PHONY` target in the `Makefile` appears in the AGENTS.md table.

#### FR-016 (P2, UX): Print Paperclip's unavailable reason once (known item)

**Finding.** `facman plan --profile agentic-factory` prints `paperclip: unavailable-here; needs the npm backend (Increment 2); Paperclip is installed by the npm backend in Increment 2`. `catalog.SelectInstall`'s reason (`internal/domain/catalog/install.go:98`) and the catalog's `hint` (`internal/adapter/catalogyaml/embedded/catalog.yaml:217`, `:221`) say the same thing, and both are joined by `internal/usecase/planning/planning.go:90`, `:103` and `internal/adapter/cli/render.go:145`.

**Acceptance criteria.**
1. Text `plan` and `scan` output show the reason once (`paperclip: unavailable-here; needs the npm backend (Increment 2)`), either by dropping the redundant catalog hint or by not joining a hint that adds nothing to the reason.
2. A golden test of headless `plan` text for the agentic-factory profile on a Linux fixture covers the line. The TUI goldens stay unchanged.

#### FR-017 (P2, docs): Close the acceptance record and keep reference material outside the archived spec

**Finding.** All 16 acceptance checkboxes in spec.md are unticked (`specs/261114-software-factory-manager/spec.md:254-269`), although status.md reports 52/52 tasks done. Nothing maps each criterion to the test that proves it. The I1-16 field check is handed to J without a procedure: `docs/product-details.md:63` only names what needs confirming. `docs/technical-details.md:136-138` and `user-docs/configuration.md:73` send readers to the spec's data dictionary for the catalog YAML format and DB tables, which become archive material in Step 7.

**Acceptance criteria.**
1. An "Acceptance evidence" table (I1-1 to I1-16, each with test names or the CI job) is added to `docs/technical-details.md`, and the spec's checkboxes are ticked in the archived copy, with I1-16 marked handed over.
2. `user-docs/field-check.md` gives J the I1-16 procedure: build or download, `facman scan --json > <host>.json` on 3 to 5 Macs, what to compare, and how to feed corrections into `catalog.yaml`. It is linked from README and user-docs/README.
3. The catalog/profile YAML format and the state DB tables are documented in `docs/technical-details.md` (or a new `docs/catalog-format.md`), and the links in the two files above point there.

#### FR-018 (P2, docs): Mark future-increment user guides on the page itself

**Finding.** `user-docs/configuration-list.md`, `hermes-setup.md`, `orca-setup.md`, `paperclip-setup.md`, `t3-setup.md` and `templates-and-validation.md` describe flags that exit 64 today. Only `user-docs/README.md` says so, and each page opens as if the feature ships.

**Acceptance criteria.**
1. Each of the six pages has, within its first three lines, a status note naming the increment that delivers it and saying that the flags exit 64 until then.
2. A docs test or script (it can share FR-015's script) asserts that the note is present.

#### FR-019 (P2, CI): Build every release target in CI and pin third-party actions

**Finding.** I1-1 requires `make build-all` to produce four binaries, but CI (`.github/workflows/ci.yml`) runs only `make build`. Cross-compilation is first exercised by the release workflow, on a tag. The actions are pinned by tag (`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v9`, `goreleaser/goreleaser-action@v7`), not by commit SHA, while the release job holds `contents: write`.

**Acceptance criteria.**
1. CI's Linux job runs `make build-all` and checks that the four binaries exist.
2. Every third-party action in both workflows is pinned to a full commit SHA with the tag in a comment, or the ADR records the decision to keep tags, with a rationale.

## Non-Functional Requirements

| Area | Requirement |
|---|---|
| Security | After the fixes, the Security NFR of spec.md holds for any catalog that passes `catalog.Validate`: there is no way to put an option or a control character into a brew argv or the Review text from data (FR-001), and no program is resolved from a relative PATH entry (FR-002). |
| Reliability | No fix changes a consented command's argv. A run can always be stopped by the user (FR-006). Concurrent first launches never fail (FR-005). |
| Performance | The spec's bounds still hold: `scan` under 5 s p95, first TUI frame under 500 ms. Probe logging (FR-008) adds under 5% to the T-F6 perf smoke numbers. |
| Observability | Every operation has a log with an exit or signal footer (FR-007). Every unknown detection has a reason (FR-008). The installer's SHA-256 is recorded (FR-009). |
| Compatibility | Report schema v1 only gains optional fields. The state DB schema is unchanged. Exit codes stay as in Appendix H. |
| Maintainability | `make check` green, coverage at least 80% with new code fully covered, the architecture rules enforced mechanically (FR-010), docs and READMEs updated in the same commit. |

## Acceptance Criteria (whole PRD)

- Every FR's acceptance criteria pass in CI on Linux and macOS (FR-003's clean-container job on Linux).
- Each fix's commit includes its regression test, and its code review is recorded in the fixes spec.
- `make check` and `make integration` are green, and `make cover` reports at least 80% overall, with new code fully covered.
- The FR-001 reproduction (`id: "-f"` in a `--catalog`) exits 3 with no brew invocation.

## Dependencies

- **Homebrew** (real installer at `https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh`, plus `brew` output shapes) for FR-003, FR-004 and FR-012.
- **GitHub Actions** runners (`ubuntu-latest` with container jobs, `macos-latest`) for FR-003, FR-004, FR-014 and FR-019.
- **J** for FR-003 AC 3 (an optional clean macOS VM run) and to accept FR-017's field-check procedure.
- Go toolchain from `go.mod` (`go 1.27`), and `golangci-lint` v2.15.0.

## Open Questions

| # | Question | Default in force |
|---|---|---|
| OQ-1 | Should the second-signal stop (FR-006) also exist in the TUI, or does the TUI's existing stop confirm suffice? | Both front ends get it; the TUI adds a second confirm. |
| OQ-2 | Exit code for a signal-killed command (FR-007): `128+signo` or a named constant? | `128+signo`, recorded in the ADR. |
| OQ-3 | Can FR-003's macOS clean-machine run be automated (for example a VM job), or does it stay a manual check for J? | Manual, recorded as an open item next to I1-16. |
| OQ-4 | Is the ID charset in FR-001 too strict for real casks or taps (for example uppercase letters in tap names)? | Use the patterns given; widen only with a catalog entry that needs it and a test. |

## Positive Observations

- Safety invariants live in the domain (`plan.Validate`) and are checked by a shared invocation-log assertion in every e2e test (I1-8, I1-9). That is the right place for them and the right enforcement.
- The installer is downloaded to a 0600 temp file, run as `/bin/bash <file>`, checked against the consented argv (`execute.go:296-307`), and removed afterwards.
- Commands run under `context.WithoutCancel`, and each operations row is written `in_progress` before its command runs, which makes crash recovery (I1-11) sound.
- The TUI consents only to the exact plan that was reviewed (`internal/adapter/tui/tui.go`, `sameActions`).
- Environment facts are injected only through `cmd/facman/wire.go` options, so Linux-no-display and WSL fixtures run on any CI runner.
