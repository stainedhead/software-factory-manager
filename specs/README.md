# Specs

Back to the [root README](../README.md). Rules and workflow: [AGENTS.md](../AGENTS.md).

Active feature specs live here; completed ones move to `specs/archive/` (via `/archive-spec`).

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

## Lifecycle

1. `/create-prd` → `/create-spec` creates `specs/YYMMDD-<feature>/`.
2. Implement with `/implm-from-spec`, updating `status.md` as you go.
3. At 100% in `status.md`, run `/archive-spec` to move it to `specs/archive/`.
