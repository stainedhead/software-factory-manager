# Hermes Agent setup (`--hermes-setup` / `-hs`)

> **Status: planned for Increment 5.** This guide describes a designed feature that does not ship yet: `-hs`/`--hermes-setup` exits 64 until then, with a message naming Increment 5. See [user-docs](README.md) for what ships today.

Use a Hermes setup spec to give every machine the same Hermes configuration: model and provider, toolsets, MCP servers, the agent's identity (`SOUL.md`), skills and profiles.

## What Hermes reads

Hermes keeps everything as plain files in `~/.hermes/`, or in `$HERMES_HOME` for a profile:

| File | Holds | Committed? |
|---|---|---|
| `config.yaml` | Model, toolsets, terminal, memory, `mcp_servers` (`${VAR}` references allowed) | ✅ yes |
| `SOUL.md` | Agent identity, put first in the system prompt | ✅ yes |
| `skills/` | Skills | ✅ yes |
| `cron/` | Scheduled jobs | ✅ optional |
| `.env` | API keys and tokens | ❌ **never**: `facman` writes it from environment variables |

Precedence: CLI arguments, then `config.yaml`, then `.env`, then built-in defaults. You can also set single values with `hermes config set KEY VAL`.

## Build one

```bash
facman template hermes -o setup/hermes/team.setup.yaml
# writes setup/hermes/team.setup.yaml plus setup/hermes/assets/{config.yaml,SOUL.md,skills/}
facman template hermes --from-machine -o setup/hermes/team.setup.yaml   # start from your current ~/.hermes (secrets excluded)
```

## Example

```yaml
# yaml-language-server: $schema=https://<schemas-host>/facman/tool-setup.v1.json
apiVersion: facman/v1
kind: ToolSetup
tool: hermes-agent
tool_version: ">=0.22 <0.23"          # the format this was written for
profile: default                       # or a named profile → hermes -p <name>
configure:
  - type: files
    from: ./assets/                    # config.yaml, SOUL.md, skills/
    to: "${HERMES_HOME:-~/.hermes}/"
    template: true                     # ${VAR} expanded at apply time
  - type: env_file
    to: "${HERMES_HOME:-~/.hermes}/.env"
    keys: [OPENROUTER_API_KEY]         # names only; values from the environment
  - type: command                      # optional one-off settings
    run: hermes config set memory.enabled true
```

`assets/config.yaml` (excerpt; ⚠ check key names against your pinned Hermes version, which the validator does for you):

```yaml
model: ${HERMES_MODEL}
mcp_servers:
  github:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env: { GITHUB_TOKEN: "${env:GITHUB_TOKEN}" }
```

## Apply

```bash
facman validate setup/hermes/team.setup.yaml --deep   # --deep runs `hermes config check` in a sandbox HERMES_HOME
facman setup -hs setup/hermes/team.setup.yaml --dry-run
facman setup -hs setup/hermes/team.setup.yaml
```

- This works on any installed Hermes, including one you installed yourself with `brew install hermes-agent`.
- **Your edits are safe.** If you change a file `facman` wrote, `facman` won't overwrite it next time. It flags the file and shows a diff.
- If Hermes isn't installed, the TUI offers to install it. Non-interactive runs exit with code 3.
