# internal/adapter/fsprobe

Conflict evidence and ownership for one path (FR-015, FR-016), implementing `conflicts.PathProbe`. Read-only.

| File | Contents |
|---|---|
| `fsprobe.go` | `Probe.Inspect(ctx, path)` → `*plan.ConflictEvidence` (nil when nothing is there): `ls -l` style line built in Go (mode, links, owner, group, size, UTC mtime, `-> target`), file type from `file -b` when available or derived in Go (`broken symbolic link to …`, `directory`, `empty`, `executable`, `regular file`), symlink target, dangling, executable bit (of the target), size, mtime. `Probe.Owner(ctx, path, entry)` → owner class and detail |

Owner rules, in order: a dangling symlink is `orphan`; the path resolved through symlinks under `{prefix}/Caskroom/<token>` or `{prefix}/Cellar|opt/<name>` is `this-package` when the entry installs that token or name, else `other-brew` (`brew cask gemini`); under `<brew or npm prefix>/lib/node_modules/<pkg>` it is `npm` (`npm @anthropic-ai/claude-code`); a match of one of the entry's other provenance globs (native, script, …) or an OS directory is `other-installer`; anything else is `orphan`. `.pkg` receipts (`pkgutil`) are not consulted in Increment 1.

The prefixes come from a `Locations` function, because the Homebrew prefix changes when facman installs Homebrew mid-run.

**Architecture.** Adapter: imports domain `catalog`, `plan` and `usecase/conflicts`, `usecase/process`. Wired in `cmd/facman`. Back to [internal/adapter](../README.md).
