# user-docs

User-facing documentation for `sfi`, the software-factory installer. It covers how to install `sfi`, give a machine or CI runner the same toolset every time, and set up each tool from files you can check into git. For engineering docs see [../docs/](../docs/).

> Status: these guides describe the **design** (v0.3). Commands, flags and schemas will be finalized during implementation. Where a tool's own format is not officially documented, it is marked ⚠.

| You want to… | Read |
|---|---|
| Install `sfi`, or run it straight from the repo | [Getting started](getting-started.md) |
| See every file and flag `sfi` reads | [Configuration](configuration.md) |
| Make a machine (or CI runner) get the same tools every time | [Configuration lists (`-cl`)](configuration-list.md) |
| Set up Hermes Agent: model, tools, MCP servers, identity, skills | [Hermes setup (`-hs`)](hermes-setup.md) |
| Load Paperclip with a company, agents, projects and skills | [Paperclip setup (`-ps`)](paperclip-setup.md) |
| Register repos, automations and skills in Orca | [Orca setup (`-os`)](orca-setup.md) |
| Point T3 Code at the right agent binaries and models | [T3 Code setup (`-ts`)](t3-setup.md) |
| Check any of these files before you commit | [Templates and validation](templates-and-validation.md) |

## The workflow for every file

1. **Start from a template:** `sfi template <kind> -o <file>`. You get an annotated file with a schema header, so VS Code autocompletes it.
2. **Edit it.** Keep secrets out of it; list only env var names.
3. **Validate it:** `sfi validate <file>`. Errors come with `file:line:col`.
4. **Preview it:** `sfi apply … --dry-run`, or `sfi setup … --dry-run`.
5. **Commit it.** The checked-in file is the consent for prompt-free runs, so review it like code.
6. **Apply it:** interactively with `sfi`, or with no prompts using `sfi apply -cl …` or `sfi setup -hs …`.

## Recommended layout for a factory's config repo

```
configs/                    # configuration lists
  build-linux.sfi.yaml
  dev-macos.sfi.yaml
setup/
  hermes/team.setup.yaml    # + assets next to it
  hermes/assets/config.yaml
  hermes/assets/SOUL.md
  paperclip/factory.setup.yaml
  paperclip/company/        # Paperclip company package
  orca/repos.setup.yaml
  t3/providers.setup.yaml
  t3/settings.fragment.json
.github/workflows/validate.yml   # runs `sfi validate configs/ setup/ --format github`
```

Update these guides whenever user-visible behaviour or configuration changes. The design they follow is [docs/design/](../docs/design/). Back to the [root README](../README.md).
