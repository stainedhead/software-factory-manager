# Paperclip setup (`--paperclip-setup` / `-ps`)

> **Status: planned for Increment 5.** This guide describes a designed feature that does not ship yet: `-ps`/`--paperclip-setup` exits 64 until then, with a message naming Increment 5. See [user-docs](README.md) for what ships today.

Use a Paperclip setup to load a **company package** (the company, its agents, projects, skills and tasks) into a Paperclip instance that `facman` runs as a service.

## What Paperclip reads

A company package is a folder (or `.zip`, or GitHub repo) of mostly markdown:

```
company/
  README.md
  COMPANY.md                 # name, goal, metadata
  .paperclip.yaml            # adapter types, budgets, env var NAMES
  agents/<name>/AGENTS.md    # each agent: role, instructions, permissions, budget
  projects/<name>/PROJECT.md
  skills/<name>/SKILL.md
  tasks/<name>/TASK.md
```

Secret **values** are never part of a package. Imported agents start **paused** (heartbeats off), so nothing runs until you turn them on.

## Build one

The easiest source of a package is an existing Paperclip:

```bash
paperclipai company export <company-id> --out ./setup/paperclip/company \
  --include company,agents,projects,skills
```

Or start from a template:

```bash
facman template paperclip -o setup/paperclip/factory.setup.yaml   # also writes setup/paperclip/company/
```

## Example

```yaml
# yaml-language-server: $schema=https://<schemas-host>/facman/tool-setup.v1.json
apiVersion: facman/v1
kind: ToolSetup
tool: paperclip
tool_version: ">=2026.7"
configure:
  - type: command
    run: >-
      paperclipai company import ./company
      --target new --new-company-name "Agentic Factory"
    once: true                       # import once; re-runs are skipped
    requires_env: [ANTHROPIC_API_KEY, OPENAI_API_KEY]
```

**Shortcut:** `-ps` also accepts a package directly:

```bash
facman setup -ps ./setup/paperclip/company
facman setup -ps github.com/<org>/factory-companies/companies/agentic --ref v1.2.0
```

## Apply

```bash
facman validate setup/paperclip/factory.setup.yaml --deep   # --deep runs `paperclipai company import … --dry-run`
facman setup -ps setup/paperclip/factory.setup.yaml
```

- `facman` starts the Paperclip service first, because the import needs a running instance, and health-checks it afterwards.
- **Network exposure** is set in the configuration list or at install time, not in the setup file. The default is localhost only.
- **Name clashes:** re-importing into an existing company uses Paperclip's conflict strategy (rename by default). To update agents in place, use `--target existing --company-id <id> --include agents` in the command.
- **Back up** `~/.paperclip/instances/default/secrets/master.key`. `facman` copies it before every upgrade.
