# Product Details

Features and behaviour are specified in the design doc until implementation starts. As each feature ships through `specs/`, its settled behaviour moves here.

| Area | Design section |
|---|---|
| Catalog of packages and how each is detected and installed | [§2, §2a](design/software-factory-installer-design.md#2-package-research-what-each-item-really-is-and-how-to-install-it) |
| Ownership states, conflicting files, tap trust | [§3](design/software-factory-installer-design.md#3-core-concept-ownership-states) |
| Startup bootstrap (Homebrew, Node/npm) | [§4a](design/software-factory-installer-design.md#4a-startup-bootstrap-homebrew-and-npm-j-2026-11-14) |
| Managed services (Paperclip) | [§6a](design/software-factory-installer-design.md#6a-managed-services-paperclip) |
| Seeding tool configuration from files | [§6b](design/software-factory-installer-design.md#6b-seeding-tool-configuration-from-files-agents-skills-settings) |
| TUI flow | [§7](design/software-factory-installer-design.md#7-tui-flow) |
| Configuration lists and unattended runs (`-cl`), per-tool setup flags | [§7a](design/software-factory-installer-design.md#7a-saved-configurations-and-unattended-runs---configuration-list) |
| Templates and validation | [§7b](design/software-factory-installer-design.md#7b-file-formats-template-validate-and-keep-them-correct-j-2026-11-14) |
| Phased plan | [§11](design/software-factory-installer-design.md#11-phased-plan) |
