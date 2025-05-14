package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea" // Bubble Tea TUI framework
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
	modifiedFiles := []string{}
	filesProcessed := 0
	var firstEncounteredError error // Stores the first non-fatal error encountered during the walk.

	// filepath.Walk efficiently traverses the directory tree.
	// The WalkFunc processes each file or directory encountered.
	walkErr := filepath.Walk(opts.Dir, func(path string, info os.FileInfo, errInWalk error) error {
		if errInWalk != nil {
			// An error occurred trying to access this path (e.g., permission denied).
			// Log it and allow filepath.Walk to continue with other items.
			accessErr := fmt.Errorf("accessing path '%s': %w", path, errInWalk)
			if firstEncounteredError == nil { // Record the first such error.
				firstEncounteredError = accessErr
			}
			// In CLI mode, this stderr print is useful. The TUI might handle logging differently or suppress it.
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Access): %v. Skipping.\n", accessErr)
			return nil // Returning nil tells filepath.Walk to continue.
		}
		if info.IsDir() {
			return nil // Skip directories, as we only operate on files.
		}

		// Check if the current filename matches the provided glob pattern.
		matched, matchErr := matchesPattern(info.Name(), opts.Pattern)
		if matchErr != nil {
			// An invalid pattern is typically a fatal error for the entire operation.
			// Returning an error here will stop filepath.Walk.
			return fmt.Errorf("invalid file pattern '%s': %w", opts.Pattern, matchErr)
		}
		if !matched {
			return nil // File does not match the pattern, skip it.
		}

		filesProcessed++ // Increment count of files that match the pattern.

		// Create a backup of the original file if the ShouldBackup option is set.
		if opts.ShouldBackup {
			if err := createBackup(path); err != nil {
				backupErr := fmt.Errorf("creating backup for '%s': %w", path, err)
				if firstEncounteredError == nil {
					firstEncounteredError = backupErr
				}
				fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Backup): %v. Continuing without backup for this file.\n", backupErr)
				// For robustness, continue processing the file even if backup fails, but log the failure.
			}
		}

		// Read the content of the current file.
		content, err := os.ReadFile(path)
		if err != nil {
			readErr := fmt.Errorf("reading file '%s': %w", path, err)
			if firstEncounteredError == nil {
				firstEncounteredError = readErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Read): %v. Skipping.\n", readErr)
			return nil // Skip this file if its content cannot be read.
		}

		// Perform the text replacement if OldText is found in the content.
		if strings.Contains(string(content), opts.OldText) {
			newContentStr := strings.ReplaceAll(string(content), opts.OldText, opts.NewText)
			// Write the modified content back to the file, preserving its original permissions.
			if err := os.WriteFile(path, []byte(newContentStr), info.Mode()); err != nil {
				writeErr := fmt.Errorf("writing modified content to '%s': %w", path, err)
				if firstEncounteredError == nil {
					firstEncounteredError = writeErr
				}
				fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformReplacement - Write): %v. Skipping modification for this file.\n", writeErr)
				return nil // Skip writing if an error occurs.
			}
			modifiedFiles = append(modifiedFiles, path) // Add to list of modified files.
		}
		return nil // Successfully processed this file, continue walking.
	})

	if walkErr != nil {
		// This error is from filepath.Walk itself (e.g., directory not found at start)
		// or a fatal error returned by the WalkFunc (like an invalid pattern).
		return modifiedFiles, filesProcessed, walkErr
	}
	// If walkErr is nil, return the first non-fatal error encountered during file processing, if any.
	return modifiedFiles, filesProcessed, firstEncounteredError
}

// PerformRestore restores files from .bak backups found in the given directory.
// It renames .bak files to their original names, overwriting existing files if necessary.
// Returns a slice of messages detailing actions taken (e.g., files restored) and any error encountered.
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
			// Skip directories and files that don't have the .bak suffix.
			return nil
		}

		originalPath := strings.TrimSuffix(path, ".bak") // Determine the original filename.
		// os.Rename will move/rename the .bak file, overwriting originalPath if it exists.
		if err := os.Rename(path, originalPath); err != nil {
			renameErr := fmt.Errorf("restoring backup '%s' to '%s': %w", path, originalPath, err)
			if firstEncounteredError == nil {
				firstEncounteredError = renameErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformRestore - Rename): %v.\n", renameErr)
			return nil // Continue with other backup files.
		}
		messages = append(messages, fmt.Sprintf("  - Restored: %s from %s", originalPath, path))
		filesRestored++
		return nil
	})

	if walkErr != nil {
		return messages, walkErr // A fatal error occurred during the directory walk.
	}
	// Append a summary message after processing all files.
	if filesRestored == 0 && firstEncounteredError == nil {
		messages = append(messages, "No .bak files found to restore in the specified directory.")
	} else if filesRestored > 0 {
		// Append summary message; individual file messages are already in the 'messages' slice.
		messages = append(messages, fmt.Sprintf("\nSuccessfully restored %d file(s).", filesRestored))
	}
	return messages, firstEncounteredError
}

// PerformClean deletes all .bak backup files found in the given directory.
// Returns a slice of messages detailing actions taken (e.g., files deleted) and any error encountered.
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
			// Skip directories and files that don't have the .bak suffix.
			return nil
		}

		if err := os.Remove(path); err != nil {
			removeErr := fmt.Errorf("deleting backup file '%s': %w", path, err)
			if firstEncounteredError == nil {
				firstEncounteredError = removeErr
			}
			fmt.Fprintf(os.Stderr, "Warning (CoreLogic - PerformClean - Remove): %v.\n", removeErr)
			return nil // Continue with other backup files.
		}
		messages = append(messages, fmt.Sprintf("  - Deleted backup: %s", path))
		filesCleaned++
		return nil
	})

	if walkErr != nil {
		return messages, walkErr // A fatal error occurred during the directory walk.
	}
	// Append a summary message.
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
	if pattern == "*" || pattern == "" { // Treat empty pattern as wildcard for user convenience.
		return true, nil
	}
	// filepath.Match can return an error if the pattern is malformed.
	return filepath.Match(pattern, filename)
}

// createBackup creates a backup copy of the source file by appending ".bak" to its name.
// It preserves the original file's permissions.
func createBackup(srcPath string) error {
	backupPath := srcPath + ".bak"
	// Feedback for individual backup creation is typically handled by the calling function
	// (e.g., PerformReplacement in core logic, or the TUI) rather than printing directly here.
	return copyFile(srcPath, backupPath)
}

// copyFile copies a file from a source path to a destination path.
// It preserves the original file's permissions.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading source file '%s' for copy: %w", src, err)
	}
	info, err := os.Stat(src) // Get file info to retrieve original permissions.
	if err != nil {
		return fmt.Errorf("getting file info for source '%s': %w", src, err)
	}
	// os.WriteFile will create the file if it doesn't exist, or truncate it if it does.
	// It uses the permissions from info.Mode().
	return os.WriteFile(dst, input, info.Mode())
}

// --- Main Function ---
// The entry point of the application. It parses command-line flags and decides
// whether to run in CLI mode or launch the interactive TUI (wizard) mode.
func main() {
	// Define command-line flags for controlling the application's behavior.
	dirFlag := flag.String("dir", ".", "Target directory for operations.")
	patternFlag := flag.String("pattern", "*", "Filename pattern (e.g., *.txt) for -replace operation.")
	oldTextFlag := flag.String("old", "", "Text to be replaced (required for -replace operation).")
	newTextFlag := flag.String("new", "", "Text to replace with (for -replace operation).")
	backupFlag := flag.Bool("backup", false, "Create .bak backup files before replacing text.")
	restoreFlag := flag.Bool("restore", false, "Restore files from .bak backups.")
	cleanFlag := flag.Bool("clean", false, "Delete all .bak backup files in the target directory.")
	wizardFlag := flag.Bool("wizard", false, "Run in interactive wizard (TUI) mode.")
	flag.Parse() // Parse the command-line flags provided by the user.

	// Determine if wizard mode should be run.
	runWizard := *wizardFlag
	// Default to wizard mode if no specific operation flags are provided by the user
	// and the -wizard flag isn't explicitly set to false. This provides a user-friendly
	// default behavior when the tool is run without any arguments.
	if !*wizardFlag && !*restoreFlag && !*cleanFlag && *oldTextFlag == "" && len(flag.Args()) == 0 {
		runWizard = true
	}

	if runWizard {
		// Launch the Bubble Tea TUI application.
		// newWizardModel() is defined in tui.go (assuming it's in the same package and 'package main').
		// tea.WithAltScreen() provides a dedicated full-screen buffer for the TUI,
		// which typically restores the original terminal content on exit.
		program := tea.NewProgram(newWizardModel(), tea.WithAltScreen())
		if _, err := program.Run(); err != nil { // Run the TUI event loop.
			fmt.Fprintf(os.Stderr, "Error running interactive wizard: %v\n", err)
			os.Exit(1) // Exit with an error code if the TUI encounters a fatal error.
		}
		os.Exit(0) // Clean exit after the TUI finishes.
	}

	// --- CLI Mode Logic ---
	// If not running in wizard mode, proceed with operations based on parsed CLI flags.
	var operationMessages []string // Stores messages to be printed to the user as feedback.
	var operationError error       // Stores any error encountered during the core operation.
	var operationPerformed bool = true // Tracks if a valid CLI operation was identified and attempted.

	if *cleanFlag {
		fmt.Fprintln(os.Stdout, "Cleaning backup files...")
		operationMessages, operationError = PerformClean(*dirFlag)
	} else if *restoreFlag {
		fmt.Fprintln(os.Stdout, "Restoring from backup files...")
		operationMessages, operationError = PerformRestore(*dirFlag)
	} else if *oldTextFlag != "" {
		// The -old flag being non-empty implies a 'replace' operation.
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

		// Construct user-friendly messages based on the outcome of the replacement.
		// Only construct these detailed messages if there wasn't a fatal error during the initial walk/pattern matching.
		// If an error occurred but some files were modified, still show those.
		if operationError == nil || (operationError != nil && len(modifiedFiles) > 0) { 
			if len(modifiedFiles) > 0 {
				operationMessages = append(operationMessages, "Successfully modified files:")
				for _, f := range modifiedFiles {
					operationMessages = append(operationMessages, fmt.Sprintf("  - %s", f))
				}
			} else if filesProcessed > 0 { // Files were processed, but none contained OldText or needed modification.
				operationMessages = append(operationMessages, "No files matched the criteria or required modification.")
			} else { // No files matched the pattern or were found in the directory.
				operationMessages = append(operationMessages, "No files found in the specified directory or matching the pattern.")
			}
		}
	} else {
		// No valid CLI operation flag was specified.
		operationPerformed = false
		if len(flag.Args()) > 0 {
			// User provided positional arguments without recognized flags.
			fmt.Fprintln(os.Stderr, "Error: Unknown arguments provided. Use flags to specify operations.")
		}
		fmt.Fprintln(os.Stderr, "No operation specified. Use -wizard for interactive mode, or provide operation flags (e.g., -old, -restore, -clean).")
		flag.Usage() // Print the standard flag usage information to help the user.
		os.Exit(1)   // Exit with an error code indicating incorrect usage.
	}

	// Output the results and status for CLI mode operations.
	if operationPerformed {
		for _, msg := range operationMessages {
			fmt.Fprintln(os.Stdout, msg) // Print informational messages.
		}
		if operationError != nil {
			// This error is typically the first non-fatal error encountered during file processing,
			// or a fatal error from the directory walk itself (e.g., bad pattern, dir not found).
			fmt.Fprintf(os.Stderr, "\nOperation completed with errors: %v\n", operationError)
			os.Exit(1) // Exit with an error code.
		} else if len(operationMessages) > 0 {
			// Only print general success if there were messages, indicating an operation was attempted and reported on.
			// Avoids printing "success" if, for example, no operation flag was given and flag.Usage() was called.
			fmt.Fprintln(os.Stdout, "\nOperation completed successfully.")
		}
	}
}
