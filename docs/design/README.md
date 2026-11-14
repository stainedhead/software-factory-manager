# docs/design

The working design for `facman`, the Agentic Software Factory Manager (v0.4: Increment 1 is built, and the points it settled are marked in the doc). It is the detailed source that the product and technical docs in [`../`](../) summarize.

| File | Purpose |
|---|---|
| [software-factory-installer-design.md](software-factory-installer-design.md) | Design v0.4: package research, ownership states, architecture, manifest and inventory formats, TUI flow, configuration lists, per-tool setup, phased plan, and open questions |

## How design changes land

1. Open a PR that edits the design doc and bumps its status line (version and date), with a short "vX.Y changes" note.
2. In the same PR, update every affected page in [`../../user-docs/`](../../user-docs/), and add an entry to [`../architecture-decision-record.md`](../architecture-decision-record.md) for any decision made or reversed.
3. If scope, audience or key decisions change, update [`../../INTENT.md`](../../INTENT.md) and [`../product-summary.md`](../product-summary.md) too.
4. Say in the PR description which decisions changed and which open questions (§12) it closes.

Once a feature moves into `specs/`, its spec becomes the place where detailed changes happen. Fold the outcome back here and into `product-details.md` and `technical-details.md` when the spec is archived.

Back to [docs](../README.md).
