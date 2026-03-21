// Package reporoot locates the root directory of the current repository.
package reporoot

import (
	"os"
	"path/filepath"
)

// Find walks up from the current working directory looking for a .cordon
// directory. If found, it returns the containing directory. If no .cordon
// directory is found, it returns the current working directory with a
// non-fatal warning string.
func Find() (root string, warn string, err error) {
	start, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	current := start
	for {
		if hasEntry(current, ".cordon") {
			return current, "", nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			// reached filesystem root
			break
		}
		current = parent
	}

	return start, "no .cordon found; run 'cordon init' to set up this project", nil
}

func hasEntry(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
