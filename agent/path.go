package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolvePath resolves a file path relative to the session's effective filesystem
// root set, preventing directory traversal outside the these dirs.
func resolvePath(cwd string, additionalDirs []string, path string) (string, error) {
	targetPath := path
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(cwd, targetPath)
	}
	targetPath = filepath.Clean(targetPath)

	// Resolve symlinks if the path exists; for non-existent paths resolve
	// the longest existing parent to keep symlink consistency with the base dirs.
	realTargetPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			realTargetPath = resolveExistingPrefix(targetPath)
		} else {
			return "", fmt.Errorf("failed to resolve path symlinks: %w", err)
		}
	}

	// Helper function to validate subpath boundaries safely
	isAllowed := func(baseDir, target string) bool {
		// Clean and evaluate symlinks for the base directory as well
		cleanBase, err := filepath.EvalSymlinks(filepath.Clean(baseDir))
		if err != nil {
			return false
		}

		rel, err := filepath.Rel(cleanBase, target)
		if err != nil {
			return false
		}

		// Reject if it climbs out (starts with "..")
		if strings.HasPrefix(rel, "..") {
			return false
		}

		return true
	}

	// Check against CWD first
	if isAllowed(cwd, realTargetPath) {
		return realTargetPath, nil
	}

	// Check against additional allowed directories
	for _, dir := range additionalDirs {
		if isAllowed(dir, realTargetPath) {
			return realTargetPath, nil
		}
	}

	return "", fmt.Errorf("path %s is not accessible", path)

}

// resolveExistingPrefix resolves symlinks on the longest existing prefix of
// a non-existent path. For example, if /var/tmp is a symlink to /private/var/tmp
// and /var/tmp/dir/sub does not exist, this returns /private/var/tmp/dir/sub.
func resolveExistingPrefix(path string) string {
	// Walk up until we find a component that exists.
	dir := path
	for dir != "/" && dir != "." {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			// Compute the relative part that was stripped and re-append it.
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return path
			}
			return filepath.Join(resolved, rel)
		}
		dir = filepath.Dir(dir)
	}
	return path
}
