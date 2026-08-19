// Package patterns reads .lnkinclude files and matches paths against them.
package patterns

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Load reads a .lnkinclude file and returns its active (non-comment,
// non-blank) lines. A missing file is treated as an empty pattern list.
func Load(path string) (patterns []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("load patterns from %s: %w", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close patterns file %s: %w", path, closeErr)
		}
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read patterns from %s: %w", path, err)
	}
	return patterns, nil
}

// Match compiles a combined list of patterns and returns whether path
// should be included. Patterns use .gitignore syntax, but a match means
// "include" rather than "ignore". Last-match-wins and ! negates.
//
// path must use '/' as the separator and be relative to the directory
// containing the .lnkinclude file (typically the project root). Because
// the path alone does not indicate whether it is a file or directory,
// Match reports a match if either possibility matches.
func Match(patterns []string, path string) (bool, error) {
	var parsed []gitignore.Pattern
	for _, p := range patterns {
		parsed = append(parsed, gitignore.ParsePattern(p, nil))
	}

	matcher := gitignore.NewMatcher(parsed)
	parts := strings.Split(path, "/")

	return matcher.Match(parts, false) || matcher.Match(parts, true), nil
}
