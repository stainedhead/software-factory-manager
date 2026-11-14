# Product Summary

**`facman`** is a terminal UI and headless CLI that gives lab machines and CI runners a consistent toolset for working in software factories. It detects what is already installed, proposes only what is missing, asks before changing anything, installs through Homebrew (on macOS, Linux and WSL) and npm, and afterwards manages only what it installed. A checked-in configuration list lets build machines get the same tools with no prompts, and per-tool setup files seed Hermes, Paperclip, Orca and T3 Code.

It is for developers on mostly single-user lab Macs, for headless Linux build machines, and for the people who define what each factory needs.

See [../INTENT.md](../INTENT.md) for scope, key decisions and non-goals, and [design/](design/) for the full design.
