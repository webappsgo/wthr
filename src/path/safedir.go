package path

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

// SafeDirPath validates a filesystem directory override (--config, --data, --cache, --log,
// --backup and their environment variables) and preserves absolute paths. Unlike the URL
// SafePath it never strips the leading slash: it rejects empty input, any control character
// (NUL, tab, CR, LF, other C0, DEL, C1) and any ".." segment in the raw input, then returns the
// cleaned path. Relative inputs are accepted and resolved against the working directory later
// (AI.md PART 5 "Apply Everywhere").
func SafeDirPath(input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}
	for _, r := range input {
		if unicode.IsControl(r) {
			return "", errors.New("invalid characters")
		}
	}
	for _, seg := range strings.Split(filepath.ToSlash(input), "/") {
		if seg == ".." {
			return "", errors.New("path traversal")
		}
	}
	return filepath.Clean(input), nil
}
