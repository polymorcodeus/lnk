// Package tracker manages the .lnk tracking file that records which files are managed.
package tracker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type RepoFormat int

const (
	FormatUnknown RepoFormat = 0 // repo not yet initialized
	FormatV1      RepoFormat = 1 // legacy: dotfiles in root
	FormatV2      RepoFormat = 2 // current: common.lnk subdir
)

// Tracker manages the .lnk tracking file that records which files are managed.
type Tracker struct {
	repoPath string
	host     string
	format   RepoFormat
}

// New creates a new Tracker.
func New(repoPath, host string, format RepoFormat) *Tracker {
	return &Tracker{
		repoPath: repoPath,
		host:     host,
		format:   format,
	}
}

// RepoPath returns the repository path.
func (t *Tracker) RepoPath() string {
	return t.repoPath
}

// LnkFileName returns the appropriate .lnk tracking file name.
func (t *Tracker) LnkFileName() (string, error) {
	if t.host == "common" {
		switch t.format {
		case FormatV2:
			return ".lnk.common", nil
		case FormatV1:
			return ".lnk", nil
		default:
			return "", fmt.Errorf("repo format not initialized, run 'lnk init' first")
		}
	}
	return ".lnk." + t.host, nil
}

// HostStoragePath returns the storage path for host-specific or common files.
func (t *Tracker) HostStoragePath() (string, error) {
	if t.host == "common" {
		switch t.format {
		case FormatV2:
			return filepath.Join(t.repoPath, "common.lnk"), nil
		case FormatV1:
			return t.repoPath, nil
		default:
			return "", fmt.Errorf("repo format not initialized, run 'lnk init' first")
		}
	}
	return filepath.Join(t.repoPath, t.host+".lnk"), nil
}

// HostStorageRelPath returns the storage path relative to the repo path for
// host-specific or common files.
func (t *Tracker) HostStorageRelPath() (string, error) {
	if t.host == "common" {
		switch t.format {
		case FormatV2:
			return filepath.Join("common.lnk"), nil
		case FormatV1:
			return ".", nil
		default:
			return "", fmt.Errorf("repo format not initialized, run 'lnk init' first")
		}
	}
	return filepath.Join(t.host + ".lnk"), nil
}

// GetManagedItems returns the list of managed files and directories from .lnk file.
func (t *Tracker) GetManagedItems() ([]string, error) {
	filename, err := t.LnkFileName()
	if err != nil {
		return []string{}, err
	}
	lnkFile := filepath.Join(t.repoPath, filename)

	content, err := os.ReadFile(lnkFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read .lnk file: %w", err)
	}

	if len(content) == 0 {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var items []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			items = append(items, line)
		}
	}

	return items, nil
}

// AddManagedItem adds an item to the .lnk tracking file.
func (t *Tracker) AddManagedItem(relativePath string) error {
	items, err := t.GetManagedItems()
	if err != nil {
		return fmt.Errorf("failed to get managed items: %w", err)
	}

	if slices.Contains(items, relativePath) {
		return nil // Already managed
	}

	items = append(items, relativePath)
	slices.Sort(items)

	return t.WriteManagedItems(items)
}

// RemoveManagedItem removes an item from the .lnk tracking file.
func (t *Tracker) RemoveManagedItem(relativePath string) error {
	items, err := t.GetManagedItems()
	if err != nil {
		return fmt.Errorf("failed to get managed items: %w", err)
	}

	newItems := slices.DeleteFunc(slices.Clone(items), func(item string) bool {
		return item == relativePath
	})

	return t.WriteManagedItems(newItems)
}

// WriteManagedItems writes the list of managed items to .lnk file.
func (t *Tracker) WriteManagedItems(items []string) error {
	filename, err := t.LnkFileName()
	if err != nil {
		return err
	}
	lnkFile := filepath.Join(t.repoPath, filename)

	content := strings.Join(items, "\n")
	if len(items) > 0 {
		content += "\n"
	}

	err = os.WriteFile(lnkFile, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write .lnk file: %w", err)
	}

	return nil
}
