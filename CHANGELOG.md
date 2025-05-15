# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
<!-- Add new changes for the next release here -->
### Changed
### Deprecated
### Removed
### Fixed
### Security

## [0.2.0] - 2025-05-15
### Added
- `go.mod` file to formalize module definition and dependencies.
### Changed
- **Core Logic & Output:**
  - Refactored core functions (`PerformReplacement`, `PerformRestore`, `PerformClean`) to return more detailed results, including counts of items affected/scanned.
  - Improved CLI output messages to be more specific about operation outcomes (e.g., number of files modified, old text not found, no files matching pattern).
  - Enhanced TUI result messages to provide clearer summaries and details based on the operation's success and the number of files affected or scanned.
- **TUI:**
  - Adjusted TUI `operationResultMsg` to carry more structured data (detail messages, items affected, files scanned) for better result display.
  - Refined logic in TUI for constructing summary messages post-operation to accurately reflect outcomes like "no files found" or "old text not found".
  - Minor layout consistency improvement in TUI list item rendering.

## [0.1.0] - 2025-05-15
### Added
- Initial release of `photonsr`.
- Core text replacement functionality (`-old`, `-new`, `-dir`, `-pattern`).
- Backup system (`-backup`) creating `.bak` files.
- Restore system (`-restore`) from `.bak` files.
- Clean system (`-clean`) to remove `.bak` files.
- Command-Line Interface (CLI) mode for all operations.
- Interactive Text User Interface (TUI) wizard mode (`-wizard` or default on no args).
- Basic input validation for directory, pattern, and required fields in TUI mode.
- Version flag (`-version`) to display application version, commit, and build date.
### Changed
- Improved error messages in TUI for invalid inputs.
- Refined TUI navigation and user experience.

[Unreleased]: https://github.com/arwahdevops/PhotonSR/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/arwahdevops/PhotonSR/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/arwahdevops/PhotonSR/releases/tag/v0.1.0
