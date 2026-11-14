# Orca setup (`--orca-setup` / `-os`)

Use an Orca setup to register repositories, scheduled automations, skills and the runtime service in Orca, so a new machine is ready to run parallel agents.

## What Orca reads

⚠ Orca keeps most settings inside the app and has **no documented settings import**. What can be automated:

| Mechanism | What it covers |
|---|---|
| `orca.yaml` (in each repo, committed) | `worktree.sharedDirectories`: gitignored folders shared into new worktrees |
| `~/.orca/keybindings.json` | Keybinding overrides |
| The Orca CLI | Repos, base refs, automations, skills, agent hooks, runtime service |

So an Orca setup is mostly a **script of CLI commands**, plus optional files.

## Build one

```bash
sfi template orca -o setup/orca/repos.setup.yaml
```

## Example

```yaml
# yaml-language-server: $schema=https://<schemas-host>/sfi/tool-setup.v1.json
apiVersion: sfi/v1
kind: ToolSetup
tool: orca
configure:
  - type: command
    run: orca repo add --path ~/src/factory-app
    once: true
  - type: command
    run: orca repo set-base-ref --repo path:~/src/factory-app --ref origin/main
  - type: command
    run: >-
      orca automations create --name nightly-triage --trigger daily --time 09:00
      --prompt "Triage new issues and open draft PRs" --repo path:~/src/factory-app
    once: true
  - type: command
    run: orca skills install <skill-name>
  - type: files
    from: ./keybindings.json
    to: ~/.orca/keybindings.json
```

Put `orca.yaml` in the application repo itself rather than in the setup, because it belongs to the repo:

```yaml
worktree:
  sharedDirectories: [node_modules, .venv]
```

## Apply

```bash
sfi validate setup/orca/repos.setup.yaml
sfi setup -os setup/orca/repos.setup.yaml
```

- Orca is a desktop app, so this runs on macOS (and Linux desktops, ⚠ if supported). On WSL and headless CI it is reported as `unavailable-here`.
- `once: true` steps are recorded and skipped on later runs, so running the setup again is safe.
