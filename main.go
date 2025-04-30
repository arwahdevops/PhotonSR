package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Parse flags
	dir := flag.String("dir", ".", "Target directory")
	pattern := flag.String("pattern", "*", "Filename pattern")
	oldText := flag.String("old", "", "Text to replace (required)")
	newText := flag.String("new", "", "Replacement text (required)")
	backup := flag.Bool("backup", false, "Create .bak backup")
	restore := flag.Bool("restore", false, "Restore from .bak files")
	clean := flag.Bool("clean", false, "Delete all .bak files")
	flag.Parse()

	// Handle clean
	if *clean {
		if err := cleanBackup(*dir); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Cleaned all .bak files")
		return
	}

	// Handle restore
	if *restore {
		if err := restoreBackup(*dir); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Restored from .bak files")
		return
	}

	// Validate required flags
	if *oldText == "" || *newText == "" {
		fmt.Println("Error: -old and -new flags are required")
		flag.Usage()
		os.Exit(1)
	}

	// Process files
	modifiedFiles := []string{}
	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !matchPattern(info.Name(), *pattern) {
			return err
		}

		// Backup file
		if *backup {
			if err := createBackup(path); err != nil {
				return err
			}
		}

		// Read content
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		// Check and replace
		if strings.Contains(string(content), *oldText) {
			modifiedFiles = append(modifiedFiles, path)
			newContent := strings.ReplaceAll(string(content), *oldText, *newText)
			if err := ioutil.WriteFile(path, []byte(newContent), info.Mode()); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Show results
	fmt.Println("Modified files:")
	for _, f := range modifiedFiles {
		fmt.Println(" ", f)
	}
}

// Helper functions
func matchPattern(filename, pattern string) bool {
	matched, _ := filepath.Match(pattern, filename)
	return matched
}

func createBackup(src string) error {
	backupPath := src + ".bak"
	return copyFile(src, backupPath)
}

func copyFile(src, dst string) error {
	input, _ := ioutil.ReadFile(src)
	return ioutil.WriteFile(dst, input, 0644)
}

func restoreBackup(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".bak") {
			return err
		}

		originalPath := strings.TrimSuffix(path, ".bak")
		if err := os.Rename(path, originalPath); err != nil {
			return err
		}
		fmt.Println("Restored:", originalPath)
		return nil
	})
}

// Clean backup function (NEW)
func cleanBackup(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".bak") {
			return err
		}

		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to delete %s: %v", path, err)
		}
		fmt.Println("Deleted:", path)
		return nil
	})
}
