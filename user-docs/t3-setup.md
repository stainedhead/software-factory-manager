# T3 Code setup (`--t3-setup` / `-ts`)

> **Status: planned for Increment 5.** This guide describes a designed feature that does not ship yet: `-ts`/`--t3-setup` exits 64 until then, with a message naming Increment 5. See [user-docs](README.md) for what ships today.

Use a T3 setup to point T3 Code at the right agent binaries and models (Codex, Claude Code and others), and to set its server options.

## What T3 Code reads

⚠ T3 Code is beta (0.1.x) and its settings format is not officially documented. The validator pins the schema to the catalog's T3 version and warns if your T3 is newer.

| Item | Where |
|---|---|
| Settings (JSON) | `settings.json` in the state directory: `$T3CODE_STATE_DIR`, default `~/.t3/userdata` (⚠ filename inferred) |
| Key settings | `providerInstances` (per agent: `binaryPath`, `customModels`, `enabled`, `homePath`), `enableProviderUpdateChecks` |
| Env vars | `T3CODE_PORT` (default 3773), `T3CODE_HOST`, `T3CODE_AUTH_TOKEN`, `T3CODE_DISABLE_AUTO_UPDATE`, `T3CODE_STATE_DIR` |

T3 has **no agent definitions of its own**. The agents are Claude Code, Codex and others, and each is configured through its own files and login (`claude auth login`, `codex login`).

## Build one

```bash
facman template t3 -o setup/t3/providers.setup.yaml      # also writes setup/t3/settings.fragment.json
facman template t3 --from-machine -o setup/t3/providers.setup.yaml
```

## Example

```yaml
# yaml-language-server: $schema=https://<schemas-host>/facman/tool-setup.v1.json
apiVersion: facman/v1
kind: ToolSetup
tool: t3-code
tool_version: ">=0.1.9 <0.2"
configure:
  - type: json_merge                 # merges keys; never replaces the whole file
    from: ./settings.fragment.json
    to: "${T3CODE_STATE_DIR:-~/.t3/userdata}/settings.json"
    backup: true
```

`settings.fragment.json`:

```json
{
  "enableProviderUpdateChecks": false,
  "providerInstances": {
    "codex": {
      "driver": "codex",
      "enabled": true,
      "config": { "binaryPath": "${BREW_PREFIX}/bin/codex", "customModels": [] }
    }
  }
}
```

## Apply

```bash
facman validate setup/t3/providers.setup.yaml
facman setup -ts setup/t3/providers.setup.yaml
```

- `facman` records exactly which keys it merged, so it can remove only those keys later. Your own settings are untouched.
- Quit T3 Code before applying. The app may rewrite the file on exit.
- T3 Code is a GUI app. On WSL it runs on the Windows host, so this setup is report-only there.
