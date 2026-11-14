# Architecture Decision Record

Newest first. Each entry gives the decision, why, and where the design covers it. Details are in [design/](design/software-factory-installer-design.md).

| Date | Decision | Why | Design |
|---|---|---|---|
| 2026-11-14 | Add a `conflict` ownership state for unowned files at a backend's install path. Never overwrite or `--force`; offer move-aside-then-install. Third-party taps need explicit, item-level trust (Homebrew 6). | Found real conflicts on a lab Mac. Homebrew 6 requires tap trust. | §3a, §3b |
| 2026-11-14 | Every file format has a template, a schema and `sfi validate`, and contract tests run on every build. | Users write these files first, and the tools' formats drift. | §7b |
| 2026-11-14 | Per-tool setup flags (`-hs`, `-ps`, `-os`, `-ts`) take a file. They run standalone or with `-cl`. | Seed agents and settings from git, not by hand. | §6b, §7a |
| 2026-11-14 | `sfi apply -cl <file>` runs with no prompts, and the file is the consent. Risky options must be written in the file. It is designed Linux-CI-first. | Build machines and CI need repeatable, auditable runs. Most `-cl` runs are on Linux. | §7a |
| 2026-11-14 | Check for Homebrew and Node/npm on every launch. The TUI prompts, and `-cl` bootstraps automatically. | Machines may have nothing installed. | §4a |
| 2026-11-14 | Run Paperclip as a managed user service, on localhost by default. Remote access only on explicit opt-in. | Paperclip is a server, and the safest exposure is the default. | §6a |
| 2026-11-14 | Assume single-user machines, where the developer owns Homebrew. | Matches the lab. Avoids admin-user indirection. | §9 |
| 2026-11-14 | Use Antigravity CLI instead of Gemini CLI. | Gemini CLI is superseded, and its formula is being disabled. | §2 |
| 2026-11-14 | Homebrew is the primary backend on macOS, Linux and WSL 2, and npm is the second. macOS is the primary target. | One backend across platforms, and most of the catalog is in Homebrew. | §2a |
| 2026-11-14 | Install only missing tools, only with consent. Manage only what `sfi` installed or the developer adopted. | Developers already own many of these machines. | §1, §3 |
| 2026-11-14 | *Proposed:* Go + Bubble Tea, YAML catalog and profiles, embedded SQLite inventory. | A single static binary works on a bare machine. | §4, §5, §6, §8 |
