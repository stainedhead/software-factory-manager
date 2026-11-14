# Specs

Back to the [root README](../README.md). Rules and workflow: [AGENTS.md](../AGENTS.md).

Active feature specs live here; completed ones move to `specs/archive/` (via `/archive-spec`).

## Active specs

None. Increment 1 and its review fixes are both complete and archived under [archive/](archive/README.md). The next increment gets its own `YYMMDD-<feature>/` directory here.

## Archived specs

| Spec | Scope | Archived |
|---|---|---|
| [archive/261114-software-factory-manager/](archive/261114-software-factory-manager/) | `facman` Increment 1 (Phase 0 + Phase 1 MVP): detect, plan and install brew packages safely, from the TUI and the CLI | 2026-11-14, 100% complete |
| [archive/261114-software-factory-manager/261114-software-factory-manager-auto-review/](archive/261114-software-factory-manager/261114-software-factory-manager-auto-review/) | Increment 1 review fixes: 26 requirements from the Step 5 code review (kept inside the Increment 1 archive) | 2026-11-14; 21 done, 3 partly done, 2 deferred (see its implementation-notes.md) |

## Naming

`YYMMDD-<feature-name>/` — the prefix is the date the spec was **created** (when `create-spec` ran), not the implementation date.

## Per-spec contents

| File | Purpose |
|---|---|
| `spec.md` | Feature specification (populated from the PRD) |
| `status.md` | Phase progress tracking — **update after every task (mandatory)** |
| `plan.md` | Implementation plan |
| `tasks.md` | Task breakdown |
| `research.md` | Research findings |
| `data-dictionary.md` | Data structures and schemas |
| `architecture.md` | Architecture and component design |
| `implementation-notes.md` | Decisions and gotchas |
| `<feature>-PRD.md` | Source PRD (moved here by `create-spec`) |
| `<feature>-status.md` | Orchestration dashboard written by `/implm-frm-prd` and the other orchestrators |
| `README.md` | What the spec covers and an index of its files |

## Lifecycle

1. `/create-prd` → `/create-spec` creates `specs/YYMMDD-<feature>/`.
2. Implement with `/implm-from-spec`, updating `status.md` as you go.
3. At 100% in `status.md`, run `/archive-spec` to move it to `specs/archive/`.
