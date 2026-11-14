# internal/adapter/detect

The detectors that do not need Homebrew (FR-010). Each implements `scan.Detector`, changes nothing, and runs programs as argv through the runner port.

| File | Contents |
|---|---|
| `path.go` | `PathDetector`: every `detect.commands` name in every PATH directory, in order; one detection per match (first = `PathWinner`), with `filepath.EvalSymlinks` and `catalog.ProvenanceOf` for provenance, and the catalog version probe run against that install (5 s, argv, regex; a version must parse, else it is unknown). Probes run concurrently (8 at a time). On macOS without the Xcode CLT (or CLT unknown), `/usr/bin/git` is never run and not reported (FR-020) |
| `bundle.go` | `BundleDetector` (macOS only): `mdfind "kMDItemCFBundleIdentifier == '<id>'"`; when Spotlight finds nothing, one scan of `/Applications` and `~/Applications` reading each `Info.plist` with `plutil -extract … raw` (binary plists included). Matching is by bundle ID only (T3 Code's "(Beta)" name). Version from `CFBundleShortVersionString`. mdfind timing out is an error (scan reports `unknown`); mdfind missing falls back to the scan |
| `npm.go` | `NPMDetector`: `npm ls -g --json --depth=0` once when npm is on PATH (else nothing), mapped by `detect.npm_package` (`paperclipai`, `@anthropic-ai/claude-code`, `@openai/codex`, `@google/gemini-cli`, …); non-zero exit with a readable list is fine, unreadable output is an error |

**Architecture.** Adapter: imports domain `catalog`, `ownership`, `version` and `usecase/scan`, `usecase/process`. Wired in `cmd/facman`. Related: [research.md RQ-5, RQ-8](../../../specs/archive/261114-software-factory-manager/research.md). Back to [internal/adapter](../README.md).
