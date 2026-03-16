package filetree

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

// ParseClipboardPaths attempts to extract valid absolute file paths
// from raw cross-platform clipboard text.
func parseClipboardPaths(data string) []string {
	var result []string

	// Normalize Windows \r\n to standard \n
	data = strings.ReplaceAll(data, "\r\n", "\n")
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle Linux text/uri-list (file:// prefixes and %20 spaces)
		if strings.HasPrefix(line, "file://") {
			parsed, err := url.Parse(line)
			if err == nil {
				result = append(result, parsed.Path)
				continue
			}
		}

		// Handle Windows "Copy as path" (strip surrounding quotes)
		if strings.HasPrefix(line, `"`) && strings.HasSuffix(line, `"`) {
			line = line[1 : len(line)-1]
		} else if strings.HasPrefix(line, `'`) && strings.HasSuffix(line, `'`) {
			line = line[1 : len(line)-1]
		}

		result = append(result, line)
	}

	return result
}

func isValidFilePath(p string) bool {
	_, err := os.Stat(p)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}
