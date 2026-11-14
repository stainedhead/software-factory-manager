# user-docs

User-facing documentation for `facman`, the **Agentic Software Factory Manager**. It covers how to install `facman`, give a machine or CI runner the same toolset every time, and set up each tool from files you can check into git. For engineering docs see [../docs/](../docs/).

> **Status:** [Getting started](getting-started.md), [Usage examples](usage-examples.md) and [Configuration](configuration.md) describe what ships today (Increment 1: the TUI, `scan`, `plan`, `apply`, `status`, `doctor`). The other guides describe features that are designed (design v0.4) but arrive in later increments: configuration lists (`-cl`) in Increment 3, templates and `validate` in Increment 4, and per-tool setup flags in Increment 5. Using those flags today exits 64 and names the increment. Where a tool's own format is not officially documented, it is marked ⚠.

| You want to… | Read |
|---|---|
| Install `facman`, or run it straight from the repo, and do a first scan, plan and install | [Getting started](getting-started.md) |
| Find a worked example: `--json` scripting, optional packages, custom catalogs, conflicts, Linux and WSL | [Usage examples](usage-examples.md) |
| Look up a flag, environment variable, state path or exit code | [Configuration](configuration.md) |
| Check detection on real lab Macs (I1-16), run the clean-machine bootstrap, or capture real `brew` output | [Field check](field-check.md) |
| *(Increment 3)* Make a machine (or CI runner) get the same tools every time | [Configuration lists (`-cl`)](configuration-list.md) |
| *(Increment 5)* Set up Hermes Agent: model, tools, MCP servers, identity, skills | [Hermes setup (`-hs`)](hermes-setup.md) |
| *(Increment 5)* Load Paperclip with a company, agents, projects and skills | [Paperclip setup (`-ps`)](paperclip-setup.md) |
| *(Increment 5)* Register repos, automations and skills in Orca | [Orca setup (`-os`)](orca-setup.md) |
| *(Increment 5)* Point T3 Code at the right agent binaries and models | [T3 Code setup (`-ts`)](t3-setup.md) |
| *(Increment 4)* Check any of these files before you commit | [Templates and validation](templates-and-validation.md) |

## The workflow for every file (Increments 3 to 5)

1. **Start from a template:** `facman template <kind> -o <file>`. You get an annotated file with a schema header, so VS Code autocompletes it.
2. **Edit it.** Keep secrets out of it; list only env var names.
3. **Validate it:** `facman validate <file>`. Errors come with `file:line:col`.
4. **Preview it:** `facman apply … --dry-run`, or `facman setup … --dry-run`.
5. **Commit it.** The checked-in file is the consent for prompt-free runs, so review it like code.
6. **Apply it:** interactively with `facman`, or with no prompts using `facman apply -cl …` or `facman setup -hs …`.

## Recommended layout for a factory's config repo

```
configs/                    # configuration lists
  build-linux.facman.yaml
  dev-macos.facman.yaml
setup/
  hermes/team.setup.yaml    # + assets next to it
  hermes/assets/config.yaml
  hermes/assets/SOUL.md
  paperclip/factory.setup.yaml
  paperclip/company/        # Paperclip company package
  orca/repos.setup.yaml
  t3/providers.setup.yaml
  t3/settings.fragment.json
.github/workflows/validate.yml   # runs `facman validate configs/ setup/ --format github`
```

Update these guides whenever user-visible behaviour or configuration changes. The design they follow is [docs/design/](../docs/design/). Back to the [root README](../README.md).
