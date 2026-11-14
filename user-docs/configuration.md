# Configuration

`facman` reads its configuration from files you check into git. Each file has its own guide:

| File | Flag | Guide |
|---|---|---|
| Configuration list (which packages, options and policy) | `--configuration-list` / `-cl` | [configuration-list.md](configuration-list.md) |
| Hermes Agent setup | `--hermes-setup` / `-hs` | [hermes-setup.md](hermes-setup.md) |
| Paperclip setup (and company package) | `--paperclip-setup` / `-ps` | [paperclip-setup.md](paperclip-setup.md) |
| Orca setup | `--orca-setup` / `-os` | [orca-setup.md](orca-setup.md) |
| T3 Code setup | `--t3-setup` / `-ts` | [t3-setup.md](t3-setup.md) |

Templates, schemas and `facman validate` are covered in [templates-and-validation.md](templates-and-validation.md). Environment variables such as `HTTPS_PROXY` and mirror overrides are described in the design doc, §9 (Fleet and lab considerations), and will be listed here once implemented.

Back to [user-docs](README.md).
