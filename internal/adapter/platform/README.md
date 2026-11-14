# internal/adapter/platform

Probes the platform facts for the bootstrap check and implements `bootstrap.PlatformProbe`.

| File | Contents |
|---|---|
| `probe.go` | `Probe` (`New(runner, Options)`), `Probe(ctx)`, `ParseOSRelease` |

| Fact | Source |
|---|---|
| OS, arch | `runtime.GOOS`, `runtime.GOARCH` (options `GOOS`, `GOARCH`) |
| WSL | `WSL_DISTRO_NAME` set, or `microsoft` in `/proc/version` |
| Display | `DISPLAY` or `WAYLAND_DISPLAY` set on Linux; always true on darwin |
| glibc | `getconf GNU_LIBC_VERSION`, then the first line of `ldd --version`; `nil` on musl or when neither answers |
| Distro | `/etc/os-release` `ID`/`ID_LIKE`: debian/ubuntu → `apt`, fedora/rhel/centos → `dnf`, else unknown |
| UID | `os.Getuid` |

Every path (`ProcVersionPath`, `OSReleasePath`), command name (`GetconfCmd`, `LddCmd`), `Getenv`, `UID` and `ReadFile` is an option, so tests use fixtures and the fake command (R-3). Missing files or commands mean "unknown", never an error.

**Architecture.** Adapter; imports domain `platform` and `version` and runs commands through the `bootstrap.CommandRunner` port. Back to [internal/adapter](../README.md).
