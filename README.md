# go-replace

CLI tool written in Go to search and replace text in files recursively with backup/restore functionality.

![GitHub](https://img.shields.io/badge/License-MIT-blue)
![Go](https://img.shields.io/badge/Go-1.19%2B-success)

## 📝 Description

**go-replace** is a powerful text replacement tool that allows you to:
- Replace text patterns in files across directories
- Create automatic `.bak` backups
- Restore original files from backups
- Clean up backup files

## ✨ Features

- 🔄 **Text Replacement** - Replace strings in all matching files
- 💾 **Backup System** - Auto-create `.bak` files before modification
- ⏮️ ** Restore System** - Revert files from backups
- 🧹 **Clean Backups** - Delete all `.bak` files
- 📁 **Pattern Matching** - Target files using wildcard patterns

## 📥 Installation

### Linux Installation

#### Method 1: From Source
```bash
# Clone repository
git clone https://github.com/arwahdevops/go-replace.git
cd go-replace

# Build and install
go build -o go-replace
sudo mv go-replace /usr/local/bin/
```

#### Method 2: Go Install
```bash
go install github.com/arwahdevops/go-replace@latest
sudo cp $HOME/go/bin/go-replace /usr/local/bin/
```

## 🚀 Usage

### Basic Command Structure
```bash
go-replace [OPTIONS] -old "OLD_TEXT" -new "NEW_TEXT"
```

### Common Options
| Flag         | Description                          |
|--------------|--------------------------------------|
| `-dir`       | Target directory (default: current)  |
| `-pattern`   | Filename pattern (e.g., *.txt)       |
| `-old`       | Text to replace (required)           |
| `-new`       | Replacement text (required)          |
| `-backup`    | Create backup files                  |
| `-restore`   | Restore files from .bak backups      |
| `-clean`     | Delete all .bak files                |

## 💡 Examples

### 1. Simple Replacement
```bash
go-replace -dir docs -pattern "*.md" -old "foo" -new "bar"
```

### 2. Safe Replacement with Backup
```bash
go-replace -dir src -old "http://" -new "https://" -backup
```

### 3. Restore Files
```bash
go-replace -dir project -restore
```

### 4. Clean Backups
```bash
go-replace -dir data -clean
```

## 📋 Important Notes

1. **Backup Safety**
   - Backup files (`*.bak`) are created in the same directory
   - Original file permissions are preserved

2. **Pattern Matching**
   - Supports standard wildcard patterns:
     - `*` matches any sequence
     - `?` matches any single character

3. **Safety First**
   - Double-check replacement patterns before execution
   - Always use `-backup` for critical operations

## 📜 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
