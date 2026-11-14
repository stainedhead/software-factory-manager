# internal/adapter/catalogyaml/embedded/profiles

The built-in profiles, one file per profile. A file must be named for its `profile:` value.

| File | Profile |
|---|---|
| `agentic-factory.yaml` | The default: includes `base-dev`, adds the agent tools; optional items are off until picked with `--with` or on the Plan screen |
| `base-dev.yaml` | Developer basics: git, gh, tmux, Visual Studio Code, iTerm2 (macOS), sqlite, node, awscli |

Only `*.yaml` files are embedded and loaded. Back to [embedded](../README.md).
