package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea" // Bubble Tea TUI framework
)

// Global variables to be injected by ldflags during the build process.
// These provide versioning information for the compiled binary.
var (
	version = "dev"     // Application version.
	commit  = "none"    // Git commit hash.
	date    = "unknown" // Build date.
	builtBy = "unknown" // Who or what built the binary (e.g., "goreleaser").
)

// --- Core Logic Functions ---
// These functions perform the actual file operations. They are called
// by both the CLI mode and the TUI mode (via tui.go).

// ReplaceOptions holds all parameters for the text replacement operation.
type ReplaceOptions struct {
	Dir          string // Target directory for the operation.
	Pattern      string // File pattern (glob) to match files for replacement.
	OldText      string // The text to be searched for and replaced.
	NewText      string // The text to replace the OldText with.
	ShouldBackup bool   // Flag indicating whether to create .bak backup files.
}

// PerformReplacement is the core function for searching and replacing text in files.
// It walks the specified directory, filters files by pattern, reads their content,
// performs the replacement, and writes the changes back.
// Returns:
//   - []string: A slice of paths to files that were actually modified.
//   - int: The total number of files that matched the pattern and were processed.
//   - error: An error if a fatal issue occurred (e.g., invalid pattern during walk setup)
//            or the first non-fatal error encountered during individual file processing.
func PerformReplacement(opts ReplaceOptions) ([]string, int, error) {
	// Critical validation: OldText cannot be empty as it would lead to unintended behavior.
	// This should ideally be caught by CLI or TUI input validation before reaching this core function.
	if opts.OldText == "" {
		return nil, 0, fmt.Errorf("text to replace (OldText) cannot be empty")
	}

	modifiedFiles := []string{}
	filesProcessed := 0
	var firstEncounteredError error // Stores the first non-fatal error encountered during the walk.

	walkErr := filepath.Walk(opts.Dir, func(path string, info os.FileInfo, errInWalk error) error {
		if errInWalk != nil {
			accessErr := fmt.Errorf("accessing path '%s': %w", path, errInWalk)
			if firstEncounteredError == nil {
				firstEncounteredError = accessErr
			}
			// Log non-fatal error and skip this path.
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Access): %v. Skipping.\n", accessErr)
			return nil
		}
		if info.IsDir() {
			return nil // Skip directories.
		}

		matched, matchErr := matchesPattern(info.Name(), opts.Pattern)
		if matchErr != nil {
			// This is a fatal error for the walk as the pattern itself is invalid.
			return fmt.Errorf("invalid file pattern '%s': %w", opts.Pattern, matchErr)
		}
		if !matched {
			return nil // File does not match the pattern.
		}

		filesProcessed++

		if opts.ShouldBackup {
			if err := createBackup(path); err != nil {
				backupErr := fmt.Errorf("creating backup for '%s': %w", path, err)
				if firstEncounteredError == nil {
					firstEncounteredError = backupErr
				}
				// Log non-fatal error and continue, but without backup for this specific file.
				fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Backup): %v. Continuing without backup for this file.\n", backupErr)
			}
		}

		content, err := os.ReadFile(path)
		if err != nil {
			readErr := fmt.Errorf("reading file '%s': %w", path, err)
			if firstEncounteredError == nil {
				firstEncounteredError = readErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Read): %v. Skipping.\n", readErr)
			return nil
		}

		if strings.Contains(string(content), opts.OldText) {
			newContentStr := strings.ReplaceAll(string(content), opts.OldText, opts.NewText)
			if err := os.WriteFile(path, []byte(newContentStr), info.Mode()); err != nil {
				writeErr := fmt.Errorf("writing modified content to '%s': %w", path, err)
				if firstEncounteredError == nil {
					firstEncounteredError = writeErr
				}
				fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Write): %v. Skipping modification for this file.\n", writeErr)
				return nil
			}
			modifiedFiles = append(modifiedFiles, path)
		}
		return nil
	})

	if walkErr != nil { // This captures errors returned directly from the WalkFunc that halt the walk (e.g., invalid pattern).
		return modifiedFiles, filesProcessed, walkErr
	}
	return modifiedFiles, filesProcessed, firstEncounteredError // Return any non-fatal errors encountered.
}

// PerformRestore restores files from .bak backups found in the given directory.
// It renames .bak files to their original names, overwriting existing files if necessary.
// Returns a slice of messages detailing actions taken and any error encountered.
func PerformRestore(dir string) ([]string, error) {
	var messages []string
	var firstEncounteredError error
	filesRestored := 0

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, errInWalk error) error {
		if errInWalk != nil {
			accessErr := fmt.Errorf("accessing '%s' during restore: %w", path, errInWalk)
			if firstEncounteredError == nil {
				firstEncounteredError = accessErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformRestore - Access): %v. Skipping.\n", accessErr)
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".bak") {
			return nil // Skip directories and non-backup files.
		}

		originalPath := strings.TrimSuffix(path, ".bak")
		if err := os.Rename(path, originalPath); err != nil {
			renameErr := fmt.Errorf("restoring backup '%s' to '%s': %w", path, originalPath, err)
			if firstEncounteredError == nil {
				firstEncounteredError = renameErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformRestore - Rename): %v.\n", renameErr)
			return nil // Continue with other files.
		}
		messages = append(messages, fmt.Sprintf("  - Restored: %s from %s", originalPath, path))
		filesRestored++
		return nil
	})

	if walkErr != nil {
		return messages, walkErr
	}
	if filesRestored == 0 && firstEncounteredError == nil {
		messages = append(messages, "No .bak files found to restore in the specified directory.")
	} else if filesRestored > 0 {
		messages = append(messages, fmt.Sprintf("\nSuccessfully restored %d file(s).", filesRestored))
	}
	return messages, firstEncounteredError
}

// PerformClean deletes all .bak backup files found in the given directory.
// Returns a slice of messages detailing actions taken and any error encountered.
func PerformClean(dir string) ([]string, error) {
	var messages []string
	var firstEncounteredError error
	filesCleaned := 0

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, errInWalk error) error {
		if errInWalk != nil {
			accessErr := fmt.Errorf("accessing '%s' during clean: %w", path, errInWalk)
			if firstEncounteredError == nil {
				firstEncounteredError = accessErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformClean - Access): %v. Skipping.\n", accessErr)
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".bak") {
			return nil // Skip directories and non-backup files.
		}

		if err := os.Remove(path); err != nil {
			removeErr := fmt.Errorf("deleting backup file '%s': %w", path, err)
			if firstEncounteredError == nil {
				firstEncounteredError = removeErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformClean - Remove): %v.\n", removeErr)
			return nil // Continue with other files.
		}
		messages = append(messages, fmt.Sprintf("  - Deleted backup: %s", path))
		filesCleaned++
		return nil
	})

	if walkErr != nil {
		return messages, walkErr
	}
	if filesCleaned == 0 && firstEncounteredError == nil {
		messages = append(messages, "No .bak files found to clean in the specified directory.")
	} else if filesCleaned > 0 {
		messages = append(messages, fmt.Sprintf("\nSuccessfully cleaned %d backup file(s).", filesCleaned))
	}
	return messages, firstEncounteredError
}

// --- Helper Functions ---

// matchesPattern checks if a filename matches the given glob pattern.
// An empty pattern or "*" is treated as a wildcard matching all files.
// Returns true if matched, false otherwise, and an error for malformed patterns.
func matchesPattern(filename, pattern string) (bool, error) {
	if pattern == "" || pattern == "*" { // Common convention: empty or "*" pattern matches all.
		return true, nil
	}
	return filepath.Match(pattern, filename)
}

// createBackup creates a backup copy of the source file by appending ".bak" to its name.
// It preserves the original file's permissions.
func createBackup(srcPath string) error {
	backupPath := srcPath + ".bak"
	return copyFile(srcPath, backupPath)
}

// copyFile copies a file from a source path to a destination path.
// It preserves the original file's permissions.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading source file '%s' for copy: %w", src, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("getting file info for source '%s': %w", src, err)
	}
	return os.WriteFile(dst, input, info.Mode())
}

// --- Main Function ---
// The entry point of the application. It parses command-line flags,
// handles the -version flag, and decides whether to run in CLI mode
// or launch the interactive TUI (wizard) mode.
func main() {
	// Define command-line flags.
	dirFlag := flag.String("dir", ".", "Target directory for operations (default: current directory).")
	patternFlag := flag.String("pattern", "*", "Filename pattern (e.g., *.txt) for -replace operation (default: *).")
	oldTextFlag := flag.String("old", "", "Text to be replaced (required for -replace operation).")
	newTextFlag := flag.String("new", "", "Text to replace with (for -replace operation).")
	backupFlag := flag.Bool("backup", false, "Create .bak backup files before replacing text.")
	restoreFlag := flag.Bool("restore", false, "Restore files from .bak backups.")
	cleanFlag := flag.Bool("clean", false, "Delete all .bak backup files in the target directory.")
	wizardFlag := flag.Bool("wizard", false, "Run in interactive wizard (TUI) mode.")
	showVersion := flag.Bool("version", false, "Show application version and exit.")

	flag.Parse() // Parse the command-line flags.

	// Handle the -version flag.
	if *showVersion {
		fmt.Printf("PhotonSR version: %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built at: %s\n", date)
		fmt.Printf("Built by: %s\n", builtBy)
		os.Exit(0)
	}

	// Determine if wizard mode should be run.
	runWizard := *wizardFlag
	// Default to wizard mode if no specific operation flags are provided
	// and -wizard flag isn't explicitly set to false.
	if !*wizardFlag && !*restoreFlag && !*cleanFlag && *oldTextFlag == "" && len(flag.Args()) == 0 {
		runWizard = true
	}

	if runWizard {
		// Launch the Bubble Tea TUI application.
		program := tea.NewProgram(newWizardModel(), tea.WithAltScreen())
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running interactive wizard: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0) // TUI handles its own exit messages.
	}

	// --- CLI Mode Logic ---
	var operationMessages []string
	var operationError error
	operationPerformed := true // Assume an operation will be performed unless explicitly set false.

	if *cleanFlag {
		fmt.Fprintln(os.Stdout, "Cleaning backup files...")
		operationMessages, operationError = PerformClean(*dirFlag)
	} else if *restoreFlag {
		fmt.Fprintln(os.Stdout, "Restoring from backup files...")
		operationMessages, operationError = PerformRestore(*dirFlag)
	} else if *oldTextFlag != "" { // -old is required for replace operation.
		fmt.Fprintln(os.Stdout, "Performing text replacement...")
		opts := ReplaceOptions{
			Dir:          *dirFlag,
			Pattern:      *patternFlag,
			OldText:      *oldTextFlag,
			NewText:      *newTextFlag,
			ShouldBackup: *backupFlag,
		}
		var modifiedFiles []string
		var filesProcessed int
		modifiedFiles, filesProcessed, operationError = PerformReplacement(opts)

		// Construct messages even if there was a non-fatal error, as some files might still have been processed.
		if len(modifiedFiles) > 0 {
			operationMessages = append(operationMessages, "Successfully modified files:")
			for _, f := range modifiedFiles {
				operationMessages = append(operationMessages, fmt.Sprintf("  - %s", f))
			}
		} else if filesProcessed > 0 && operationError == nil {
			// Files were processed (matched pattern) but none contained OldText or required modification.
			operationMessages = append(operationMessages, "No files matched the criteria or required modification (old text not found).")
		} else if operationError == nil {
			// No files processed and no error (e.g., directory empty or no files matched pattern).
			operationMessages = append(operationMessages, "No files found in the specified directory or matching the pattern.")
		}
		// If operationError is not nil, it will be printed later.
	} else {
		// No primary operation flag (-clean, -restore, -old) was given.
		operationPerformed = false
		if len(flag.Args()) > 0 { // Check for unknown arguments.
			fmt.Fprintln(os.Stderr, "Error: Unknown arguments provided. Use flags to specify operations.")
		}
		// If -version was already handled, or if user explicitly passed -wizard=false without other ops.
		fmt.Fprintln(os.Stderr, "No operation specified. Use -wizard for interactive mode, or provide operation flags (e.g., -old, -restore, -clean, -version).")
		flag.Usage() // Print usage information.
		os.Exit(1)
	}

	// Output results and status for CLI mode operations.
	if operationPerformed {
		for _, msg := range operationMessages {
			fmt.Fprintln(os.Stdout, msg)
		}
		if operationError != nil {
			fmt.Fprintf(os.Stderr, "\nOperation completed with errors: %v\n", operationError)
			os.Exit(1)
		} else if len(operationMessages) > 0 || (*cleanFlag || *restoreFlag) { // Check if any message or non-replace op ran
			fmt.Fprintln(os.Stdout, "\nOperation completed successfully.")
		}
	}
}
