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
var (
	version = "dev"     // Application version.
	commit  = "none"    // Git commit hash.
	date    = "unknown" // Build date.
	builtBy = "unknown" // Who or what built the binary (e.g., "goreleaser").
)

// --- Core Logic Functions ---

// ReplaceOptions holds all parameters for the text replacement operation.
type ReplaceOptions struct {
	Dir          string // Target directory for the operation.
	Pattern      string // File pattern (glob) to match files for replacement.
	OldText      string // The text to be searched for and replaced.
	NewText      string // The text to replace the OldText with.
	ShouldBackup bool   // Flag indicating whether to create .bak backup files.
}

// PerformReplacement is the core function for searching and replacing text in files.
// Returns:
//   - []string: A slice of paths to files that were actually modified.
//   - int: The total number of files that matched the pattern and were processed (read attempt).
//   - error: An error if a fatal issue occurred or the first non-fatal error.
func PerformReplacement(opts ReplaceOptions) ([]string, int, error) {
	if opts.OldText == "" {
		return nil, 0, fmt.Errorf("text to replace (OldText) cannot be empty")
	}

	modifiedFiles := []string{}
	filesProcessed := 0 // Counts files that matched the pattern and were attempted to be read
	var firstEncounteredError error

	walkErr := filepath.Walk(opts.Dir, func(path string, info os.FileInfo, errInWalk error) error {
		if errInWalk != nil {
			accessErr := fmt.Errorf("accessing path '%s': %w", path, errInWalk)
			if firstEncounteredError == nil {
				firstEncounteredError = accessErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Access): %v. Skipping.\n", accessErr)
			return nil
		}
		if info.IsDir() {
			return nil
		}

		matched, matchErr := matchesPattern(info.Name(), opts.Pattern)
		if matchErr != nil {
			return fmt.Errorf("invalid file pattern '%s': %w", opts.Pattern, matchErr)
		}
		if !matched {
			return nil
		}

		filesProcessed++ // Increment when a file matches the pattern and will be processed

		if opts.ShouldBackup {
			if err := createBackup(path); err != nil {
				backupErr := fmt.Errorf("creating backup for '%s': %w", path, err)
				if firstEncounteredError == nil {
					firstEncounteredError = backupErr
				}
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

	if walkErr != nil {
		return modifiedFiles, filesProcessed, walkErr
	}
	return modifiedFiles, filesProcessed, firstEncounteredError
}

// PerformRestore restores files from .bak backups.
// Returns:
//   - []string: Slice of messages detailing individual actions taken.
//   - int: Number of files successfully restored.
//   - error: The first non-fatal error encountered or walk error.
func PerformRestore(dir string) ([]string, int, error) {
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
			return nil
		}

		originalPath := strings.TrimSuffix(path, ".bak")
		if err := os.Rename(path, originalPath); err != nil {
			renameErr := fmt.Errorf("restoring backup '%s' to '%s': %w", path, originalPath, err)
			if firstEncounteredError == nil {
				firstEncounteredError = renameErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformRestore - Rename): %v.\n", renameErr)
			return nil
		}
		messages = append(messages, fmt.Sprintf("  - Restored: %s from %s", originalPath, path))
		filesRestored++
		return nil
	})

	if walkErr != nil {
		return messages, filesRestored, walkErr
	}
	// Summary message for "no files found" is now primarily handled by the caller (CLI/TUI)
	// based on filesRestored count and error state. This function returns the raw data.
	// However, if this function were to be used standalone, a "no files found" message here might be useful.
	// For now, we keep it lean. The TUI/CLI will explicitly check filesRestored.
	if filesRestored == 0 && firstEncounteredError == nil && walkErr == nil {
		// This explicit message can be useful if this function is called directly
		// and the caller doesn't build its own summary.
		messages = append(messages, "No .bak files found to restore in the specified directory.")
	}
	return messages, filesRestored, firstEncounteredError
}

// PerformClean deletes all .bak backup files.
// Returns:
//   - []string: Slice of messages detailing individual actions taken.
//   - int: Number of files successfully cleaned.
//   - error: The first non-fatal error encountered or walk error.
func PerformClean(dir string) ([]string, int, error) {
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
			return nil
		}

		if err := os.Remove(path); err != nil {
			removeErr := fmt.Errorf("deleting backup file '%s': %w", path, err)
			if firstEncounteredError == nil {
				firstEncounteredError = removeErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformClean - Remove): %v.\n", removeErr)
			return nil
		}
		messages = append(messages, fmt.Sprintf("  - Deleted backup: %s", path))
		filesCleaned++
		return nil
	})

	if walkErr != nil {
		return messages, filesCleaned, walkErr
	}
	if filesCleaned == 0 && firstEncounteredError == nil && walkErr == nil {
		messages = append(messages, "No .bak files found to clean in the specified directory.")
	}
	return messages, filesCleaned, firstEncounteredError
}

// --- Helper Functions ---

// matchesPattern checks if a filename matches the given glob pattern.
func matchesPattern(filename, pattern string) (bool, error) {
	if pattern == "" || pattern == "*" {
		return true, nil
	}
	return filepath.Match(pattern, filename)
}

// createBackup creates a backup copy of the source file.
func createBackup(srcPath string) error {
	backupPath := srcPath + ".bak"
	return copyFile(srcPath, backupPath)
}

// copyFile copies a file from src to dst, preserving permissions.
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
func main() {
	dirFlag := flag.String("dir", ".", "Target directory for operations (default: current directory).")
	patternFlag := flag.String("pattern", "*", "Filename pattern (e.g., *.txt) for -replace operation (default: *).")
	oldTextFlag := flag.String("old", "", "Text to be replaced (required for -replace operation).")
	newTextFlag := flag.String("new", "", "Text to replace with (for -replace operation).")
	backupFlag := flag.Bool("backup", false, "Create .bak backup files before replacing text.")
	restoreFlag := flag.Bool("restore", false, "Restore files from .bak backups.")
	cleanFlag := flag.Bool("clean", false, "Delete all .bak backup files in the target directory.")
	wizardFlag := flag.Bool("wizard", false, "Run in interactive wizard (TUI) mode.")
	showVersion := flag.Bool("version", false, "Show application version and exit.")

	flag.Parse()

	if *showVersion {
		fmt.Printf("PhotonSR version: %s\n", version)
		fmt.Printf("Commit: %s\n", commit)
		fmt.Printf("Built at: %s\n", date)
		fmt.Printf("Built by: %s\n", builtBy)
		os.Exit(0)
	}

	runWizard := *wizardFlag
	if !*wizardFlag && !*restoreFlag && !*cleanFlag && *oldTextFlag == "" && len(flag.Args()) == 0 {
		runWizard = true
	}

	if runWizard {
		program := tea.NewProgram(newWizardModel(), tea.WithAltScreen())
		if _, err := program.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running interactive wizard: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// --- CLI Mode Logic ---
	var operationMessages []string
	var operationError error
	var itemsAffected int // Number of files modified, restored, or cleaned
	var filesScanned int  // For replacement: number of files matching pattern that were scanned
	operationPerformed := true
	actionVerb := ""

	if *cleanFlag {
		actionVerb = "cleaned"
		fmt.Fprintln(os.Stdout, "Cleaning backup files...")
		operationMessages, itemsAffected, operationError = PerformClean(*dirFlag)
	} else if *restoreFlag {
		actionVerb = "restored"
		fmt.Fprintln(os.Stdout, "Restoring from backup files...")
		operationMessages, itemsAffected, operationError = PerformRestore(*dirFlag)
	} else if *oldTextFlag != "" {
		actionVerb = "modified"
		fmt.Fprintln(os.Stdout, "Performing text replacement...")
		opts := ReplaceOptions{
			Dir:          *dirFlag, Pattern:      *patternFlag,
			OldText:      *oldTextFlag, NewText:      *newTextFlag,
			ShouldBackup: *backupFlag,
		}
		var modifiedFilePaths []string
		modifiedFilePaths, filesScanned, operationError = PerformReplacement(opts)
		itemsAffected = len(modifiedFilePaths)

		// Prepend detailed modification messages
		if itemsAffected > 0 {
			detailedMessages := []string{"Successfully modified files:"}
			for _, f := range modifiedFilePaths {
				detailedMessages = append(detailedMessages, fmt.Sprintf("  - %s", f))
			}
			// Prepend these messages to any messages returned by PerformReplacement (e.g., "no files found" if itemsAffected is 0)
			operationMessages = append(detailedMessages, operationMessages...)
		}

		// Handle cases where no files were modified but files were scanned
		if operationError == nil && itemsAffected == 0 {
			if filesScanned > 0 {
				// This message might already be part of operationMessages from PerformReplacement if it handles this logic.
				// Let's ensure it's clear.
				hasNoMatchMsg := false
				for _, msg := range operationMessages {
					if strings.Contains(msg, "Old text not found") || strings.Contains(msg, "No files matched the criteria") {
						hasNoMatchMsg = true
						break
					}
				}
				if !hasNoMatchMsg {
					operationMessages = append(operationMessages, "Old text not found in any matching files, or files were already up-to-date.")
				}
			} else { // filesScanned == 0
				hasNoFilesFoundMsg := false
				for _, msg := range operationMessages {
					if strings.Contains(msg, "No files found") {
						hasNoFilesFoundMsg = true
						break
					}
				}
				if !hasNoFilesFoundMsg {
					operationMessages = append(operationMessages, "No files found matching the pattern in the specified directory.")
				}
			}
		}

	} else {
		operationPerformed = false
		if len(flag.Args()) > 0 {
			fmt.Fprintln(os.Stderr, "Error: Unknown arguments provided. Use flags to specify operations.")
		}
		fmt.Fprintln(os.Stderr, "No operation specified. Use -wizard for interactive mode, or provide operation flags (e.g., -old, -restore, -clean, -version).")
		flag.Usage()
		os.Exit(1)
	}

	// Output results and status for CLI mode operations.
	if operationPerformed {
		for _, msg := range operationMessages {
			// Avoid printing duplicate "no files found" messages if already handled by core logic.
			// This simple check might need refinement if messages become more complex.
			isSummaryMsgFromCore := (strings.Contains(msg, "No .bak files found") || strings.Contains(msg, "No files found")) && itemsAffected == 0
			if !(isSummaryMsgFromCore && actionVerb != "modified") { // For replace, detail messages are more critical
				fmt.Fprintln(os.Stdout, msg)
			}
		}

		if operationError != nil {
			fmt.Fprintf(os.Stderr, "\nOperation completed with errors: %v\n", operationError)
			if itemsAffected > 0 {
				fmt.Fprintf(os.Stderr, "However, %d file(s) were successfully %s before the error occurred.\n", itemsAffected, actionVerb)
			}
			os.Exit(1)
		} else {
			// Success messages
			if itemsAffected > 0 {
				fmt.Fprintf(os.Stdout, "\nSuccessfully %s %d file(s).\n", actionVerb, itemsAffected)
			} else if actionVerb == "modified" && filesScanned > 0 {
				// Message about "Old text not found..." should have been in operationMessages
				fmt.Fprintln(os.Stdout, "\nOperation completed. No files required changes.")
			} else if (actionVerb == "cleaned" || actionVerb == "restored") && itemsAffected == 0 {
				// Message about "No .bak files found..." should have been in operationMessages
				// if the core function added it.
				// If operationMessages is empty, means the core func didn't add it.
				if len(operationMessages) == 0 || (len(operationMessages) == 1 && operationMessages[0] == "") {
					fmt.Fprintf(os.Stdout, "\nNo .bak files found to %s.\n", strings.TrimSuffix(actionVerb, "ed"))
				} else {
					fmt.Fprintln(os.Stdout, "\nOperation completed.")
				}
			} else if actionVerb == "modified" && filesScanned == 0 {
				// "No files found matching pattern"
                 fmt.Fprintln(os.Stdout, "\nOperation completed.")
            } else {
				fmt.Fprintln(os.Stdout, "\nOperation completed successfully.") // General fallback
			}
		}
	}
}
