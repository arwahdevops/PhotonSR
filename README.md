# PhotonSR

CLI and Interactive TUI tool written in Go to search and replace text in files recursively with backup/restore functionality.

![GitHub License](https://img.shields.io/badge/License-MIT-blue)
![Go Version](https://img.shields.io/badge/Go-1.24.2%2B-success)
![Mode](https://img.shields.io/badge/Mode-CLI%20%26%20TUI-informational)

## 📝 Description

**PhotonSR** is a powerful and user-friendly text replacement tool that allows you to:
- Replace text patterns in files across directories using command-line arguments.
- **NEW:** Use an interactive Text User Interface (TUI) wizard for a guided experience.
- Create automatic `.bak` backups before modifying files.
- Restore original files from these backups.
- Clean up (delete) all backup files in a directory.

The TUI mode is particularly helpful for users who prefer a step-by-step process over typing out all command-line flags.

## ✨ Features

- 🔄 **Text Replacement** - Replace strings in all matching files.
- 🧙 **Interactive Wizard Mode (TUI)** - A user-friendly, step-by-step terminal interface for all operations (built with Bubble Tea).
- 💾 **Backup System** - Auto-create `.bak` files before modification.
- ⏮️ **Restore System** - Revert files from `.bak` backups.
- 🧹 **Clean Backups** - Delete all `.bak` files.
- 📁 **Pattern Matching** - Target files using wildcard patterns (e.g., `*.txt`, `src/**/*.go`).
- ✅ **Input Validation** - Enhanced checks for inputs like directory paths and file patterns in TUI mode.
- 🛡️ **Error Handling** - Improved feedback for both CLI and TUI modes.

## 📥 Installation

### Prerequisites
- `curl` or `wget` installed on your system.
- `tar` installed (for extracting the downloaded archive).
- For standard Linux/macOS systems, you might need `sudo` privileges if installing to a system directory like `/usr/local/bin`.

### Easy Installation (Recommended)

You can install PhotonSR with a single command using either `curl` or `wget`. The script will automatically detect your OS and architecture, download the latest version, and install it to a standard location (e.g., `/usr/local/bin` or `$PREFIX/bin` on Termux).

**Using `curl`:**
```bash
sh -c "$(curl -fsSL https://raw.githubusercontent.com/arwahdevops/PhotonSR/main/install.sh)"
```

**Using `wget`:**
```bash
sh -c "$(wget -qO- https://raw.githubusercontent.com/arwahdevops/PhotonSR/main/install.sh)"
```

After installation, you might need to open a new terminal session or source your shell's configuration file (e.g., `source ~/.bashrc`, `source ~/.zshrc`) for the `photonsr` command to be available.

### Manual Installation (from Source)

If you prefer to build from source:

1.  **Prerequisites**:
    *   Go 1.24.2 or newer (update this if your `go.mod` specifies a newer version).
    *   Git

2.  **Clone and Build**:
    ```bash
    # Clone repository
    git clone https://github.com/arwahdevops/PhotonSR.git
    cd PhotonSR

    # Build
    go build -o photonsr ./cmd
    ```

3.  **Install Manually**:
    Move the compiled `photonsr` binary to a directory in your `PATH`.
    ```bash
    # Example: install to /usr/local/bin (may require sudo)
    sudo mv photonsr /usr/local/bin/
    # Or for Termux:
    # mv photonsr $PREFIX/bin/
    ```

## 🚀 Usage

`PhotonSR` can be run in two modes: **CLI Mode** (using command-line flags) or **Wizard Mode** (interactive TUI).

### 🧙 Wizard Mode (Interactive TUI)

This is the recommended mode for ease of use. It will guide you through the steps for any operation.

To start the wizard mode, simply run:
```bash
photonsr
```
Or explicitly:
```bash
photonsr -wizard
```

The wizard will prompt you for the action (Replace, Restore, Clean), target directory, text, patterns, and other necessary options.

### 🖥️ CLI Mode

Use command-line flags for scripting or if you prefer direct commands.

#### Basic Command Structure
```bash
photonsr [OPTIONS] -old "OLD_TEXT" -new "NEW_TEXT"
photonsr [OPTIONS] -restore
photonsr [OPTIONS] -clean
```

#### Common Options
| Flag         | Alias | Description                                       | Applicable To       |
|--------------|-------|---------------------------------------------------|---------------------|
| `-wizard`    |       | Run in interactive wizard (TUI) mode.             | (Mode selection)    |
| `-dir`       |       | Target directory (default: current directory `.`) | All operations      |
| `-pattern`   |       | Filename pattern (e.g., `*.txt`, `main.*`)        | Replace             |
| `-old`       |       | Text to replace (required for replace operation)  | Replace             |
| `-new`       |       | Replacement text (required for replace operation) | Replace             |
| `-backup`    |       | Create `.bak` backup files before modification    | Replace             |
| `-restore`   |       | Restore files from `.bak` backups                 | Restore             |
| `-clean`     |       | Delete all `.bak` files in the target directory   | Clean               |
| `-version`   |       | Show application version and exit.                | (Global)            |


**Note:** If `photonsr` is run without any operation flags (`-old`, `-restore`, `-clean`) and `-wizard` is not specified, it will default to launching the **Wizard Mode**.

## 💡 Examples

### 1. Simple Replacement (CLI)
Replaces "foo" with "bar" in all `.md` files within the `docs` directory.
```bash
photonsr -dir docs -pattern "*.md" -old "foo" -new "bar"
```

### 2. Safe Replacement with Backup (CLI)
Replaces "http://" with "https://" in all files (default pattern `*`) within the `src` directory, creating backups first.
```bash
photonsr -dir src -old "http://" -new "https://" -backup
```

### 3. Restore Files (CLI)
Restores original files from `.bak` files in the `project` directory.
```bash
photonsr -dir project -restore
```

### 4. Clean Backups (CLI)
Deletes all `.bak` files from the `data` directory.
```bash
photonsr -dir data -clean
```

### 5. Using Wizard Mode
For any of the above operations, or if you're unsure about the flags, simply run:
```bash
photonsr
```
...and follow the on-screen prompts.

## 📋 Important Notes

1.  **Backup Safety**:
    *   Backup files (e.g., `filename.txt.bak`) are created in the same directory as the original file.
    *   Original file permissions are preserved on both the modified file and the backup file.
2.  **Pattern Matching**:
    *   Uses standard Go `filepath.Match` glob patterns:
        *   `*` matches any sequence of non-separator characters.
        *   `?` matches any single non-separator character.
        *   `[set]` matches any one character in set.
        *   `[^set]` matches any one character not in set.
        *   For more complex needs, consider tools with regex support.
3.  **Case Sensitivity**:
    *   Text replacement is case-sensitive by default. "Foo" will not match "foo".
4.  **Safety First**:
    *   **Always double-check** your replacement text (`-old` and `-new`), target directory (`-dir`), and file patterns (`-pattern`) before execution, especially in CLI mode.
    *   It is **highly recommended** to use the `-backup` flag (or confirm backup creation in wizard mode) for critical operations. Test on non-critical data first if unsure.

## 📜 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

Contributions, issues, and feature requests are welcome! Feel free to check [issues page](https://github.com/arwahdevops/photonsr/issues).
(Consider adding a CONTRIBUTING.md if you have specific guidelines)

## 🙏 Acknowledgements

-   [Bubble Tea](https://github.com/charmbracelet/bubbletea) and the Charmbracelet ecosystem for making TUI development in Go delightful.
