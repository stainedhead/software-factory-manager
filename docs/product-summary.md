# Product Summary

**`facman`**, the **Agentic Software Factory Manager**, is a terminal UI and headless CLI that gives lab machines and CI runners a consistent toolset for working in software factories. It detects what is already installed, proposes only what is missing, asks before changing anything, installs through Homebrew (on macOS, Linux and WSL) and npm, and afterwards manages only what it installed. A checked-in configuration list lets build machines get the same tools with no prompts, and per-tool setup files seed Hermes, Paperclip, Orca and T3 Code.

It is for developers on mostly single-user lab Macs, for headless Linux build machines, and for the people who define what each factory needs.

## Where it stands

**Increment 1 (MVP) is built.** It is one static binary for macOS, Linux and WSL 2 that does the following:

- Scans the 30-entry built-in catalog and reports each tool's ownership state and how it got there.
- Plans the exact Homebrew commands for a profile (`agentic-factory`, `base-dev`), and refuses to install over files it does not own.
- Installs after explicit consent, from the TUI or `facman apply`, and bootstraps Homebrew when it is missing.
- Records every action, with who consented, in a local audit log.
- Reports status and diagnoses conflicts and tap trust (`facman doctor`), with `--json` output on every command.

Next come management (upgrade, uninstall, adopt, the npm backend and Paperclip as a service) in Increment 2, and prompt-free configuration lists for CI in Increment 3. Formats and validation follow in Increment 4, and per-tool setup from Increment 5. See [product-details.md](product-details.md) for what ships and what is still to come.

See [../INTENT.md](../INTENT.md) for scope, key decisions and non-goals, [design/](design/) for the full design, and [../user-docs/getting-started.md](../user-docs/getting-started.md) to try it.
