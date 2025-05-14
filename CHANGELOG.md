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

## [0.2.0] - 2025-05-14

### Added
- Initial release of `go-replace`.
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

[Unreleased]: https://github.com/arwahdevops/go-replace/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/arwahdevops/go-replace/releases/tag/v0.1.0
