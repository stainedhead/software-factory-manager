# Field check: real Macs and a clean machine

This is the procedure for the two checks that CI cannot do: the **I1-16 field check** (detection rules on 3 to 5 real lab Macs) and the **clean-machine bootstrap** (`facman` installs Homebrew itself, then a profile). They are for the factory owner, J, and anyone who maintains the catalog. Back to [user-docs](README.md).

Why these are manual: CI's macOS runners already have Homebrew, so they cannot show the first-install path. Running the real Homebrew installer on a machine you care about changes it for good, so it belongs on a disposable VM or a freshly wiped Mac. The automated tests cover the same flows against fakes (spec I1-5) and the read-only half against the runner's Homebrew (I1-14, plus the `brew info` contract test).

## What you need

- `facman` built from this repo (`make build`, giving `bin/facman`) or downloaded from GitHub Releases. Check it with `facman --version`.
- For part 2: a clean macOS VM (or a wiped Mac) with no Homebrew at `/opt/homebrew` or `/usr/local`, signed in as a normal admin user (not root).
- About 10 minutes per Mac for part 1, and 30 to 60 minutes for part 2 (the Homebrew installer and the downloads take most of it).

## Part 1: I1-16 detection check on 3 to 5 lab Macs

`scan` changes nothing on the machine. It only writes its state directory (`~/Library/Application Support/facman/` on macOS; see [State and logs](configuration.md#state-and-logs)).

1. On each Mac, run:

   ```bash
   facman scan --json > "$(hostname -s).json"
   facman scan > "$(hostname -s).txt"
   ```

2. Read each `.txt` (it is the same data as the JSON). For every package the Mac really has, check three things:
   - **State.** It should be `present` (installed by Homebrew from the catalog's formula or cask), `foreign` (installed some other way, such as a `.dmg`, the vendor's script, or the OS), or `duplicate` (more than one install). A tool you know is installed but shows `absent` is a detection gap.
   - **Provenance.** The `provenance` of each detection (`brew`, `brew-cask`, `native`, `npm`, `script`, `app-bundle`, `os`) should match how it was really installed. An `unknown` provenance means a missing glob in the catalog.
   - **Version.** An empty version for a tool that prints one means its `detect.version` regex does not match that version's output.
3. Note any `warning` findings, such as `path-relative-entry` (a `.` or empty entry in `PATH`), `brew-shellenv`, or a conflict.
4. Pay particular attention to the items the build could not confirm (docs/product-details.md, *Known limits*): the app bundle IDs of the GitHub Copilot app, T3 Code and Orca, and the shape of `brew trust` output.
5. Feed corrections into [`internal/adapter/catalogyaml/embedded/catalog.yaml`](../internal/adapter/catalogyaml/embedded/catalog.yaml):
   - a wrong or missing path: add or fix the entry's `detect.provenance` glob (templates `{brew_prefix}`, `{npm_prefix}` and `~`);
   - a wrong app: fix `detect.bundle_ids` (read the real one with `mdls -name kMDItemCFBundleIdentifier /Applications/<App>.app`);
   - an empty version: fix `detect.version.regex` against the real `--version` output.

   Every catalog value must still pass the allow-lists in [configuration.md](configuration.md#catalog-directories). Run `make check`, then commit the change with the `.json` files that motivated it (strip anything private first).

## Part 2: clean-machine bootstrap (review FR-003)

Run this on a disposable VM, because it really installs Homebrew.

1. Confirm the machine is clean: `ls /opt/homebrew /usr/local/bin/brew` both fail, and `command -v brew` prints nothing.
2. Run the read-only commands first. Both must report Homebrew as not installed and install nothing:

   ```bash
   facman scan
   facman plan --profile agentic-factory
   ```

3. Run the install with one small profile from this repo:

   ```bash
   facman apply --profile integration --catalog testdata/catalogs/integration
   ```

   The prompt must list the Homebrew installer first, as `/bin/bash <temp file>` marked **unverified script**, and then `brew install hello`. Answer `y`. The installer asks for your password itself; `facman` never sees it.
4. Record the result. Copy this block into the fixes spec's `implementation-notes.md` (entry "macOS clean-machine run"), or into an issue:

   ```text
   date:               <UTC date>
   macOS version:      <sw_vers -productVersion>
   Homebrew version:   <brew --version, first line>
   facman version:     <facman --version>
   apply exit code:    <echo $? right after apply>
   packages rows:      <sqlite3 ~/Library/"Application Support"/facman/state.db "select id, state, bootstrap, backend from packages">
   ```

   Expected: exit code `0`; a `homebrew` row with state `managed` and `bootstrap = 1`; a `hello` row with state `managed`.
5. Run the same `apply` again. It must say there is nothing to install and exit `0`.
6. Optionally, run `facman apply --profile base-dev` to exercise casks (iTerm2, Visual Studio Code) and the sudo hand-off, then check that their `packages` rows are `managed` with backend `brew-cask`.

## Part 3: capture real `brew` output for the test fixtures

The brew adapter's test fixtures were written by hand ([testdata/README.md](../internal/adapter/brew/testdata/README.md)). To replace them with real output, run the read-only contract test on any Mac with Homebrew (it runs `brew info --json=v2`, `brew tap` and `brew trust` and changes nothing):

```bash
mkdir -p /tmp/brew-capture
FACMAN_CAPTURE_DIR=/tmp/brew-capture go test -v -tags integration -run TestIntegrationBrewContract ./internal/adapter/brew/
```

It fails if a field `facman` reads has changed type. The files in `/tmp/brew-capture` can then replace the ones in `internal/adapter/brew/testdata/` (keep the test expectations in step), with the Homebrew version recorded in that folder's README.

## If something goes wrong

- Each command `facman apply` ran has a log under `logs/` in the state directory, named by time, operation and package. The path is printed in the Summary.
- `facman status` shows what `facman` recorded; `facman doctor` re-checks conflicts and tap trust.
- Exit codes are listed in [configuration.md](configuration.md#exit-codes).
