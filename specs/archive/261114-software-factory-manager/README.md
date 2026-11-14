# specs/archive/261114-software-factory-manager

Active spec for **Increment 1 (Phase 0 + Phase 1 MVP)** of `facman`, the Agentic Software Factory Manager: detect, plan and install brew packages safely, from the TUI and the CLI. Later increments get their own spec directories.

Back to [specs/archive/](../README.md) and [specs/](../../README.md). Source of intent: [INTENT.md](../../../INTENT.md); rules: [AGENTS.md](../../../AGENTS.md).

| File | Purpose |
|---|---|
| [software-factory-manager-PRD.md](software-factory-manager-PRD.md) | Source PRD for the whole product (reviewed 2026-11-14). Its *Delivery plan* defines Increment 1 |
| [spec.md](spec.md) | Increment 1 specification: in-scope and deferred FRs, NFRs, edge cases, acceptance criteria I1-1 to I1-16 |
| [status.md](status.md) | Phase progress, updated after every task (mandatory) |
| [software-factory-manager-status.md](software-factory-manager-status.md) | Dev-flow orchestration dashboard (`/implm-frm-prd` steps and timings) |
| [research.md](research.md) | Research questions and findings (brew JSON, tap trust, installer, platform probes, testing) |
| [data-dictionary.md](data-dictionary.md) | Domain entities, ports, SQLite schema, catalog YAML, report schema v1 |
| [architecture.md](architecture.md) | Package map across domain/usecase/adapter, data flow, sequences, decisions AD-1 to AD-16 |
| [plan.md](plan.md) | Workstreams, parallelism, critical path, testing strategy, rollout |
| [tasks.md](tasks.md) | 52 TDD-ordered tasks in workstreams A to H |
| [implementation-notes.md](implementation-notes.md) | Decisions (D-*, review R-*), edge cases, deviations, measurements, lessons |

How it fits: this directory drives the code under `cmd/facman` and `internal/`. When Increment 1 is complete in `status.md`, `/archive-spec` moves it to `specs/archive/`.
