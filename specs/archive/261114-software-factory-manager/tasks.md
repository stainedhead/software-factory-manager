# Tasks: software-factory-manager, Increment 1

**Feature:** software-factory-manager (`facman`)
**Date:** 2026-11-14
**Status:** Reviewed (2026-11-14), ready for implementation

## Progress Summary

**48 / 52 tasks complete** (A 11/11, B 4/4, C 5/5, D 8/8 + T-D9, E 9/9, F 6/6, G 5/5, H 0/4)

Format: **ID — title** · *Depends on* · *Estimate* · **Test first** (the failing test to write before any code) · **Done when** (acceptance). Every task also updates the README of each folder it creates or changes, and `status.md` when it completes (mandatory). Estimates are focused engineering hours. Workstreams, order and parallelism are in [plan.md](plan.md#phase-breakdown-workstreams).

---

## Workstream A — Foundation (sequential, first)

**✅ T-A1 — Makefile targets.** *Depends:* — · *Est:* 0.5 h
- **Test first:** a `cmd/facman` test (or script in CI) is not practical for make targets; verify by running the targets. Add `build: CGO_ENABLED=0 go build …`, `build-all` (darwin/linux × amd64/arm64 into `bin/<os>-<arch>/facman`), `integration` (`go test -race -tags integration ./...`).
- **Done when:** `make build`, `make build-all`, `make integration` work locally; `file bin/linux-amd64/facman` shows a static binary; root README and AGENTS-mirrored command table in README updated.

**✅ T-A2 — `domain/version`.** *Depends:* — · *Est:* 1 h
- **Test first:** table tests for `Parse` (`2.1.4`, `v24.11.0`, `1.7.1-rc1`, `aws-cli/2.17.0` after regex, garbage), `Compare`, `AtLeast` (unknown never satisfies).
- **Done when:** 100% coverage; README.

**✅ T-A3 — `domain/platform`.** *Depends:* T-A2 · *Est:* 1.5 h
- **Test first:** `Key()` for darwin/linux/wsl; `GlibcBelow("2.39")` for 2.35/2.39/nil; `BootstrapReport.Fatal(cmd)` returns the fatal findings per command (UID 0 fatal for all; `linux-prereqs` and `brew-prefix-owner` fatal for `apply`/TUI Run only).
- **Done when:** types per data-dictionary §2.2 and §2.9; README.

**✅ T-A4 — `domain/failure`.** *Depends:* — · *Est:* 1 h
- **Test first:** `ExitCode()` for each kind (64, 3, 4, 1, 2); `Worst(errs…)` precedence usage > bootstrap > validation > operation; `errors.As` through wrapping.
- **Done when:** README.

**✅ T-A5 — `domain/catalog` model and `Validate`.** *Depends:* T-A3, T-A4 · *Est:* 3 h
- **Test first:** table tests: duplicate IDs, unknown `requires`, unknown choice member, missing `kind`, unknown backend, `trust` ≠ `item`, requires cycle (`a→b→a`), all reported together as one validation error.
- **Done when:** types per data-dictionary §2.3–2.4; README.

**✅ T-A6 — `domain/catalog` install selection.** *Depends:* T-A5 · *Est:* 2 h
- **Test first:** `SelectInstall(entry, platform, implemented)`: darwin cask; linux cask; wsl cli inherits linux; wsl gui → `unavailable-here` + winget hint; linux gui without display → `unavailable-here`; `platforms: [darwin]` on linux → not offered (excluded, not shown as wanted); `npm` only → `unavailable-here` "needs the npm backend (Increment 2)"; `report` → `unavailable-here` with hint.
- **Done when:** README updated.

**✅ T-A7 — `domain/catalog` profile resolution.** *Depends:* T-A5 · *Est:* 3 h
- **Test first:** `Resolve`: `agentic-factory` includes `base-dev`; include cycle → validation error; two profiles union; required beats optional; same pin twice OK; different pins → error naming both profiles; `--with` optional selects; `--with` unknown → error; `--with container-runtime` → error listing members; `--with colima` picks the member; two members of one group → error; `FromProfiles` populated.
- **Done when:** README updated.

**✅ T-A8 — `domain/plan` model, ordering and invariants.** *Depends:* T-A5 · *Est:* 3 h
- **Test first:** topological order (requires first, ties by catalog order, deterministic); `Validate` rejects argv with `--force`/`--overwrite`, `brew trust stablyai/orca` (whole tap), `upgrade`/`uninstall`/`untap`/`untrust`, any action for a non-`absent` package; `Consent.String()` = `user@2026-11-14T19:00:00Z`; `ExcludeWithDependents` removes a package and its transitive dependents with reasons.
- **Done when:** types per data-dictionary §2.8; README.

**✅ T-A9 — `domain/inventory`.** *Depends:* T-A8 · *Est:* 1 h
- **Test first:** status transitions allowed (`in_progress → succeeded|failed|interrupted`, nothing out of terminal states); `Record` validation (`state` only managed/adopted).
- **Done when:** README.

**✅ T-A10 — `domain/ownership` classification.** *Depends:* T-A6, T-A8, T-A9 · *Est:* 4 h
- **Test first (I1-3):** one table producing every state: `absent`, `foreign` (brew install, no record), `foreign` + `OSBundled` (`/usr/bin/jq`), `managed` (record + detected via recorded backend), `adopted` (record state adopted), `drifted` (record, not found; record brew-cask, found native), `duplicate` (brew + native claude, winner = first on PATH), `conflict` (absent + conflict), `unavailable-here`; `satisfies_if` node 24.11 via nvm → `foreign` satisfying; node 22 via nvm → `foreign` not satisfying (shown, never planned in Increment 1, hint to install `node@24`); `reject_if` awscli 1.x → `foreign` not satisfying; `Incomplete` when a relevant detector is `unknown`; same file seen by PATH and brew detectors is not a duplicate.
- **Done when:** 100% branch coverage; README.

**✅ T-A11 — `fakecmd` and `internal/testkit`.** *Depends:* — · *Est:* 4 h
- **Test first:** `testkit` tests: building `fakecmd`, linking it as `brew`, running `brew info --json=v2 --installed` returns the fixture; `brew install jq` adds `jq` to state and later `brew info` reflects it; a fixture rule can set exit code, stdout, stderr, sleep; every call appends `{argv, env_names, cwd}` to the log; `testkit.AssertSafeInvocations(t, log)` fails on `--force`, `--overwrite`, `upgrade`, `uninstall`, whole-tap trust, or `HOMEBREW_NO_REQUIRE_TAP_TRUST` in env.
- **Done when:** `testdata/fakecmd/main.go` builds with `go build` (Go ignores `testdata` for `./...`, so it is not part of the product); `internal/testkit/README.md` says it is test-only.

---

## Workstream B — Catalog v1 data and loader (parallel with C, D, E)

**✅ T-B1 — YAML decoder.** *Depends:* T-A5 · *Est:* 3 h
- **Test first:** decode a small catalog and profile into domain types; unknown key → `catalog.yaml:12:5 packages.jq.detekt: unknown field`; wrong type → line/col; order of `packages` preserved; `service:` and `policy:` accepted opaquely.
- **Done when:** `adapter/catalogyaml` README.

**✅ T-B2 — Embedded catalog v1.** *Depends:* T-B1, T-A6 · *Est:* 4 h
- **Test first:** loads and validates; contains every FR-040 ID plus choice members and helpers; FR-041 defaults; `orca` uses `stablyai/orca/orca` with `tap` and `trust: item`; `hermes-agent` (never `hermes`); `iterm2` `platforms: [darwin]`; `paperclip` npm only; `sign_in_required` on the five entitlement tools; `node` `node@24` with `satisfies_if min_version 24.11` and no `link`.
- **Done when:** `embedded/catalog.yaml` follows PRD Appendix E and design §5 (detect commands, bundle IDs, provenance globs, version probes).

**✅ T-B3 — Embedded profiles.** *Depends:* T-B2, T-A7 · *Est:* 1.5 h
- **Test first (I1-4):** `agentic-factory` resolves to its packages plus `base-dev` = git, gh, tmux, visual-studio-code, iterm2 (darwin), sqlite, node, awscli; optional items unselected; `--with gcloud-cli` selects it.
- **Done when:** `embedded/profiles/{agentic-factory,base-dev}.yaml`.

**✅ T-B4 — `--catalog <dir>` source.** *Depends:* T-B1 · *Est:* 1.5 h
- **Test first:** dir with `catalog.yaml` + `profiles/` loads and replaces the embedded catalog; missing dir/file → validation error (exit 3) with path; `https://…` or `git@…` → usage error (exit 64) "git catalogs arrive in a later increment"; `Revision` = `dir:<abs>@<sha256>` stable across runs.
- **Done when:** README updated.

---

## Workstream C — Infrastructure adapters (parallel with B, D, E)

**✅ T-C1 — `adapter/execrunner`.** *Depends:* T-A4 · *Est:* 3 h
- **Test first:** runs argv (no shell) and returns exit code/stdout/stderr; tees output to a log writer; timeout kills the child and returns a timeout error; env passed is `base + overrides` with every `HOMEBREW_NO_REQUIRE_TAP_TRUST*` removed; interactive mode attaches stdin/stdout/stderr of the parent; log header lists env var **names** only; system `Clock` and `UserInfo` (`os/user`, falling back to `$USER`).
- **Done when:** README.

**✅ T-C2 — `adapter/platform`.** *Depends:* T-A3 · *Est:* 2 h
- **Test first:** fixture files for `/proc/version` (WSL and not), `/etc/os-release` (ubuntu, debian, fedora, rhel, alpine → unknown), `getconf` and `ldd` outputs via `fakecmd` (2.35, 2.39, missing); env for display/WSL.
- **Done when:** paths and command names are constructor parameters; README.

**✅ T-C3 — `adapter/statedir`.** *Depends:* — · *Est:* 2.5 h
- **Test first:** macOS path under `~/Library/Application Support/facman`; Linux path honours `XDG_STATE_HOME` and defaults to `~/.local/state/facman`; log name `20261114-190000-7-gh.log`; `PruneLogs` deletes files older than 90 days only inside `logs/`; `Lock` is exclusive across processes (child process test) and `TryLock` reports the holder PID; unwritable dir → bootstrap failure (exit 4).
- **Done when:** README.

**✅ T-C4 — `adapter/sqlite` open and migrations.** *Depends:* T-A9 · *Est:* 2.5 h
- **Test first:** fresh DB gets migration 1 tables (data-dictionary §4) and `meta.schema_version=1`, `machine_id`; re-open is a no-op; `schema_version=99` → bootstrap error "written by a newer facman"; garbage file → bootstrap error naming the path, file untouched.
- **Done when:** pure Go (`modernc.org/sqlite`), `CGO_ENABLED=0 go test` passes; README.

**✅ T-C5 — `adapter/sqlite` inventory repository.** *Depends:* T-C4 · *Est:* 3 h
- **Test first:** implements the `Inventory` port: `StartOperation`/`FinishOperation` round-trip with `commands_json`; `MarkInterrupted` changes only `in_progress` rows and returns them; `UpsertRecord`; `SaveDetections` keeps the last 10 scans per package; `RecordTapTrust`; times stored as RFC 3339 UTC; readable by `sqlite3` CLI when present (skip if absent).
- **Done when:** README updated.

---

## Workstream D — Detection and Homebrew adapters (parallel with B, C, E)

**✅ T-D1 — Locate brew and `shellenv`.** *Depends:* T-C1 (runner shape), T-A3 · *Est:* 2 h
- **Test first:** brew on PATH found; not on PATH but at an injected known prefix found with `OnPATH=false`; none → not found; `brew shellenv` output (`export HOMEBREW_PREFIX="…"; export PATH="…"`) parsed into a map without a shell; prefix owned by another UID flagged (temp dir + injected owner lookup).
- **Done when:** `adapter/brew` README.

**✅ T-D2 — Parse `brew info --json=v2`.** *Depends:* T-D1 · *Est:* 3 h
- **Test first:** fixtures captured from real Homebrew (formulae with/without linked keg, keg-only, casks with `app`, `binary` (plain and with `target`), `pkg`, `installer`), the empty case, invalid JSON → `unknown` detection with reason; `brew trust` and `brew tap` listing parsers (unknown format → "not trusted").
- **Done when:** fixtures in `internal/adapter/brew/testdata/` with a note on the Homebrew version they came from.

**✅ T-D3 — Brew detector.** *Depends:* T-D2 · *Est:* 2 h
- **Test first:** one `brew info --json=v2 --installed` call (asserted in the fakecmd log) with `HOMEBREW_NO_AUTO_UPDATE=1`; maps installed formulae/casks to catalog entries by install `id` (tap items by full name); provenance `brew`/`brew-cask`; version; 10 s timeout → `unknown`.
- **Done when:** implements `Detector`.

**✅ T-D4 — `brew-formula` and `brew-cask` backends.** *Depends:* T-D2, T-A8 · *Est:* 3 h
- **Test first:** `InstallCommands`: `brew install jq`; `brew install --cask claude-code`; Orca: `brew tap stablyai/orca` and `brew trust --cask stablyai/orca/orca` as separate `TapTrustStep`s, omitted when `TapState` says tapped/trusted, then `brew install --cask stablyai/orca/orca`; `NeedsSudo` for casks with `pkg`/`installer`; `LinkPaths`: cask `binary` → `{prefix}/bin/<name>`, `app` → `/Applications/<App>.app`, formula → `detect.commands` under `{prefix}/bin`; no output ever contains `--force`/`--overwrite`/`link`.
- **Done when:** implements `Backend`; README.

**✅ T-D5 — PATH detector.** *Depends:* T-A10, T-C1 · *Est:* 3 h
- **Test first:** temp dirs with symlinks: brew Caskroom path → `brew-cask`; `~/.local/share/claude/versions/…` → `native`; npm global path → `npm`; nvm path → `nvm`; `/usr/bin/jq` → `os`; two `claude` on PATH → both detections, first is `PathWinner`; version probe via argv with 5 s timeout and regex; garbage output → version unknown; **`git` on darwin with `xcode-select -p` failing: `/usr/bin/git` is never executed** (asserted through the fakecmd log) and not reported present.
- **Done when:** implements `Detector`; README.

**✅ T-D6 — App-bundle detector.** *Depends:* T-A10 · *Est:* 2.5 h
- **Test first:** `mdfind` (fakecmd) returns a path → found with version from `plutil`; `mdfind` empty → fallback scan of injected app dirs reading `Info.plist` via `plutil`; T3 matched by bundle ID despite "(Beta)" name; not darwin → detector returns nothing; timeout → `unknown`.
- **Done when:** implements `Detector`.

**✅ T-D7 — npm detector.** *Depends:* T-A10 · *Est:* 1.5 h
- **Test first:** `npm ls -g --json --depth=0` fixture maps `paperclipai`, `@anthropic-ai/claude-code`, `@openai/codex` to catalog entries (via `detect.npm_package`); npm absent → no detections, no error; bad JSON → `unknown`.
- **Done when:** implements `Detector`.

**✅ T-D8 — `adapter/fsprobe` conflict evidence and owners.** *Depends:* T-A8 · *Est:* 3 h
- **Test first (I1-9 unit part):** real temp files: dangling symlink → `IsSymlink`, `Dangling`, target, `orphan`; regular non-executable file; symlink into `{prefix}/Caskroom/other/…` → `other-brew` with detail; into `{prefix}/Cellar/<this pkg>` → `this-package`; into `{prefix}/lib/node_modules/@anthropic-ai/claude-code` → `npm`; into `~/.local/share/claude` → `other-installer`; missing path → nil; `LsLine` formatted in Go (mode, size, mtime, `-> target`); `file -b` used when available, Go-derived type otherwise.
- **Done when:** implements `PathProbe`; README.

---

**✅ T-D9 — Detection-only `gemini-cli` entry (gap from C).** *Depends:* T-B2 · *Est:* 1 h
- **Test first:** catalog entry `gemini-cli` (`gemini`, npm `@google/gemini-cli`) with `superseded_by: antigravity-cli`, no install block, offered nowhere; validation rejects an unknown `superseded_by`, an install block on a superseded entry, and a profile that wants it; scan reports a found Gemini CLI `foreign` with an info finding `superseded` (T-E3).
- **Done when:** PRD FR-040 notes and Appendix C Scan satisfied; README updated.

## Workstream E — Use cases (parallel with B, C, D; start first after A)

**✅ T-E1 — Ports and fakes.** *Depends:* T-A2–T-A10 · *Est:* 2 h · **Lands in workstream A's commit (R-5)** so B, C, D and E can start in parallel against fixed ports.
- **Test first:** compile-time checks that the in-memory fakes in each use case's `_test.go` satisfy the ports (data-dictionary §3).
- **Done when:** port interfaces declared in their consuming packages; `internal/usecase/README.md` lists them.

**✅ T-E2 — `usecase/bootstrap`.** *Depends:* T-E1 · *Est:* 3 h
- **Test first (I1-5 unit part):** UID 0 → fatal `uid0` with fix; brew missing → warning `brew-missing` (not fatal for scan/plan/apply); brew at prefix not on PATH → found + `shellenv` applied; CLT missing on darwin → warning; Linux prerequisites missing on ubuntu → finding with `sudo apt-get update && sudo apt-get install -y build-essential procps curl file git`, fatal for apply; fedora → dnf command; unknown distro + missing prereqs → fatal for apply; glibc 2.35 → `glibc-old` warning; Node 24.11 via nvm → satisfies, `foreign`; prefix owned by another user → fatal for apply.
- **Done when:** README.

**✅ T-E3 — `usecase/scan`.** *Depends:* T-E1 · *Est:* 3 h
- **Test first:** detectors run concurrently (fake detectors block until all started); one detector exceeding the timeout (fake clock/context) → `unknown` with reason and `Incomplete` on its packages, scan still completes; reconcile: with lock → `MarkInterrupted`, AD-8 (interrupted install now detected via its backend → record `managed`; not detected → stays absent); without lock → no writes, warning; detections snapshot saved; every catalog entry classified.
- **Done when:** README.

**✅ T-E4 — `usecase/conflicts`.** *Depends:* T-E1 · *Est:* 2 h
- **Test first:** for each selected `absent` brew item, `LinkPaths` checked via `PathProbe`; `this-package` → no conflict; anything else → `Conflict` with evidence, owner, failing command (`brew install --cask claude-code`) and fix hint; Orca before tap: paths from catalog `detect.commands` and bundle name (D-9).
- **Done when:** README.

**✅ T-E5 — `usecase/planning`.** *Depends:* T-E3, T-E4 · *Est:* 4 h
- **Test first (I1-6, I1-10, part of I1-5/I1-9):** brew missing → first action `bootstrap-homebrew` (installer command, `Unverified`, `NeedsSudo`), every brew action depends on it; Orca → `tap`, `trust`, `install` actions in that order; already tapped/trusted → omitted; dependencies first (`awscli` before `session-manager-plugin` when `--with session-manager-plugin`); `conflict` → excluded with dependents; `unavailable-here`, `foreign`, `duplicate`, `drifted`, `managed`, `adopted`, `Incomplete` → no action (exclusion reasons where wanted); choice group satisfied when any member present; empty plan; `plan.Validate` passes on every produced plan.
- **Done when:** README.

**✅ T-E6 — `usecase/execute`: operations log and results.** *Depends:* T-E5 · *Est:* 4 h
- **Test first (I1-7 unit part):** for each action: `StartOperation(in_progress)` is written **before** the runner is called (fake records the order), `FinishOperation` after with exit code and log path; `consent_by` on every row; success → `UpsertRecord(managed)` (homebrew with `bootstrap=true`, backend `script`), trust → `RecordTapTrust`; failure → dependents skipped, independent actions continue, result exit 1; no automatic retry; dry run → runner never called, no inventory writes; decline → nothing written; `NeedsSudo` commands run interactive.
- **Done when:** README.

**✅ T-E7 — `usecase/execute`: bootstrap re-plan and cancellation.** *Depends:* T-E6 · *Est:* 3 h
- **Test first:** after `bootstrap-homebrew` succeeds: brew re-located, `shellenv` refreshed in the runner env, re-scan; a package now present is dropped; a new conflict is excluded (exit 1); no command outside the consented plan is ever run (assert set inclusion); installer download fails 3 times → bootstrap failure exit 4, nothing else runs; cancel signal during a command → that command finishes and is recorded, remaining actions not started, exit 1.
- **Done when:** README updated.

**✅ T-E8 — `usecase/doctor` and `usecase/status`.** *Depends:* T-E3, T-E4 · *Est:* 2.5 h
- **Test first:** doctor checks every catalog brew item available on this platform (not only selected), plus catalog tap items' trust state → findings; clean → exit 0; info-only → 0; conflict → 2; needed untrusted tap (`stablyai/orca` not trusted) → 2 with the exact `brew trust --cask stablyai/orca/orca` command as fix; status lists managed/adopted/foreign/drifted/duplicate with provenance.
- **Done when:** README.

**✅ T-E9 — `usecase/summary`.** *Depends:* T-E6 · *Est:* 1.5 h
- **Test first:** per-package result lines; "not installed: conflict / dependency failed / detection incomplete / unavailable here (reason)"; "installed, sign-in required" for installed `sign_in_required` packages; profile lines (`eval "$(<prefix>/bin/brew shellenv)"`, `export PATH="<prefix>/opt/node@24/bin:$PATH"`) when brew was installed or is not on PATH, or node@24 was installed.
- **Done when:** README.

---

## Workstream F — CLI, report, wiring, end-to-end (parallel with G)

**✅ T-F1 — `adapter/report` schema v1.** *Depends:* E · *Est:* 3 h
- **Test first:** encoder output for scan, plan, apply, status and doctor results validates against the embedded `schema/report.v1.json` (santhosh-tekuri/jsonschema in tests); a deliberately broken document fails; optional fields omitted; `consent_by` null on dry run.
- **Done when:** schema per data-dictionary §7; README.

**✅ T-F2 — `adapter/cli` command tree and exit codes.** *Depends:* T-F1 · *Est:* 4 h
- **Test first (I1-12):** `--version`/`--about` output unchanged (existing tests ported); `scan|plan|apply|status|doctor` registered; `upgrade`, `uninstall`, `adopt`, `forget`, `restore`, `setup`, `service`, `export`, `config`, `template`, `validate` → 64 with "arrives in a later increment"; unknown command/flag → 64; `--profile` with no value → 64; `plan`/`apply` without `--profile` → 64; `--set` → 64; `--catalog https://…` → 64; failure kinds → exit codes; `--json` writes only JSON to stdout, messages to stderr.
- **Done when:** README.

**✅ T-F3 — Headless consent, dry run and signals.** *Depends:* T-F2 · *Est:* 3 h
- **Test first:** non-terminal stdin → 64 with TUI hint; pty: prompt text `Proceed? [y/N]` after the command list; `y`/`Y`/`yes` → run; `n`, empty, EOF, `Ctrl+C` at prompt → exit 0, no rows; `--dry-run` never prompts; first SIGINT during Run → stop after current command, exit 1.
- **Done when:** README updated.

**✅ T-F4 — `cmd/facman` wiring.** *Depends:* T-F2, B, C, D · *Est:* 2 h
- **Test first:** `run(args, stdio)` with `defaultOptions()` still passes existing `--version`/`--about` tests; `newApp(options)` builds every adapter from options (platform probe, UID, known prefixes, app dirs, `/etc/os-release` and `/proc/version` paths, installer URL, clock, user, stdio) (R-3); no-arg with non-terminal stdout prints `--about` text + hint, exit 0 (AD-14); root `--profile`/`--with`/`--catalog` reach the TUI (R-6).
- **Done when:** `main.go` + `wire.go` only wire; `cmd/facman/README.md` updated.

**✅ T-F5 — End-to-end tests.** *Depends:* T-F4, T-A11 · *Est:* 6 h
- **Test first:** `cmd/facman/e2e_test.go`, in-process with fixtures: I1-2 (scan `--json` schema-valid, read-only invocations, < 5 s), I1-4 (`--catalog`, unknown ID exit 3, pins exit 3), I1-5 (no brew: scan/plan report and install nothing; apply lists installer first, decline changes nothing; consent with an `httptest` installer that creates a fake brew in the injected prefix → `homebrew` row `managed`, `bootstrap = 1`, later commands get `HOMEBREW_PREFIX` from `shellenv` (R-1); UID 0 via injected UID → 4; linuxbrew prefix found; glibc warning in `--dry-run`), Linux-no-display and WSL fixtures: GUI items `unavailable-here`, WSL `winget` hint (R-9), `doctor` clean → 0 and untrusted Orca tap → 2 (R-2), `apply --json` keeps the prompt on stderr (R-8), I1-6, I1-7 (pty consent, op rows, second apply runs nothing exit 0), I1-9 (dangling `claude` symlink, exit 1, doctor exit 2), I1-10, I1-12; I1-11 as a subprocess (fake `brew install` sleeps; test SIGKILLs facman after the `in_progress` row appears; next run marks it interrupted and finishes); `testkit.AssertSafeInvocations` after every test (I1-8).
- **Done when:** all green on Linux and macOS without Homebrew access.

**✅ T-F6 — Performance smoke tests.** *Depends:* T-F5 · *Est:* 1 h
- **Test first:** `--version` in-process < 50 ms and as a built binary < 1 s (generous to avoid flakes; the 100 ms target is measured manually); full-catalog fixture scan < 5 s; first TUI `View()` < 500 ms.
- **Done when:** measured numbers recorded in implementation-notes.

---

## Workstream G — TUI (parallel with F)

**✅ T-G1 — Model, screens and keys.** *Depends:* E · *Est:* 3 h
- **Test first:** state machine: Scan → (`Enter`) Plan → (`Enter`) Review → (`Enter` = consent) Run → Summary; `Esc` back from Plan/Review/Summary keeps selections; `q` quits from any screen except Run with no runner call; `?` help overlay; `Tab` shows "Manage and Files arrive in a later increment"; header shows profile `agentic-factory`; golden test of the Scan screen.
- **Done when:** `adapter/tui` README; Bubble Tea major recorded in ADR (S-3).

**✅ T-G2 — Plan screen.** *Depends:* T-G1 · *Est:* 3 h
- **Test first (I1-13):** grouped by state; required missing pre-checked; optional unchecked; `space` toggles only selectable rows (`absent`, not conflict, not unavailable); foreign/duplicate/drifted not selectable; `unavailable-here` greyed with reason (Paperclip: npm backend); `i` shows source, why needed, which profile asked, and conflict evidence; `/` filters; choice group single-select (or none); goldens.
- **Done when:** README updated.

**✅ T-G3 — Review screen.** *Depends:* T-G2 · *Est:* 2 h
- **Test first:** every command in order with sudo and unverified markers, dependency order, tap and trust steps, glibc and other warnings; only `Enter` here consents (consent recorded `user@time`); empty selection → "Nothing to install" and `Enter` goes to Summary without consent; goldens.
- **Done when:** README updated.

**✅ T-G4 — Run screen.** *Depends:* T-G3 · *Est:* 3 h
- **Test first:** `Progress` messages update per-item spinner/status and a log tail; a `NeedsSudo` command arrives as an `execRequest` message and the model returns a `tea.ExecProcess` command built by an injected exec factory, whose callback writes the exit code to the request's reply channel (R-4, AD-16); `Ctrl+C` asks for confirmation, then stops after the current command; `q` ignored during Run; goldens with deterministic spinner frame.
- **Done when:** README updated.

**✅ T-G5 — Summary screen.** *Depends:* T-G4, T-E9 · *Est:* 1.5 h
- **Test first:** per-item results, "not installed: conflict", sign-in next steps, shell profile lines, log paths; golden; full-flow golden test Scan → Summary with fixtures.
- **Done when:** README updated.

---

## Workstream H — Integration, CI, docs, quality gate (last)

**T-H1 — Real-Homebrew integration tests.** *Depends:* F · *Est:* 3 h
- **Test first:** `cmd/facman/integration_test.go` (`//go:build integration`): skip if UID 0 or no brew (PATH or known prefixes); read-only `scan --json` (schema-valid), `plan --profile agentic-factory`, `doctor` (exit 0 or 2) with a temp state dir; if `FACMAN_INTEGRATION_MUTATE=1` and darwin: skip if `hello` already installed, else `apply --profile integration --catalog testdata/catalogs/integration` through a pty answering `y`, assert `packages` row `managed`, cleanup `brew uninstall hello`.
- **Done when:** passes locally without brew (skips) and on CI macOS (I1-14).

**T-H2 — CI wiring.** *Depends:* T-H1 · *Est:* 1 h
- **Test first:** n/a (workflow). Change the integration step to `make integration`; set `FACMAN_INTEGRATION_MUTATE: "1"` only when `runner.os == 'macOS'`; keep `make build` and smoke test; update `.github/README.md`.
- **Done when:** CI green on both OSes.

**T-H3 — Documentation.** *Depends:* F, G · *Est:* 3 h
- **Test first:** n/a. Update root `README.md` (status, commands, quick start), `docs/technical-details.md` (real components, data flow, ports, dependencies), `docs/product-details.md` (Increment 1 features), `docs/architecture-decision-record.md` (AD-2, AD-3, AD-5, AD-8, AD-9, AD-10, AD-14 and Bubble Tea version), `user-docs/getting-started.md` and `configuration.md` (state paths, `--catalog`, exit codes); every new folder README present and linked from its parent.
- **Done when:** I1-15 docs part satisfied.

**T-H4 — Quality gate.** *Depends:* T-H1–T-H3 · *Est:* 1 h
- **Test first:** n/a. `make check` green; `make cover` ≥ 80% overall and ≥ 90% domain/usecase; `go mod tidy` clean; README audit script (`find internal cmd -type d` vs `README.md`) clean.
- **Done when:** I1-15 satisfied; status.md Phase 5 complete.
