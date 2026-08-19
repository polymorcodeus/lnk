// Package scope abstracts the bidirectional path translation between a file's
// "live" location on the local filesystem and its "storage" location within
// the lnk repository.
package scope

import (
	"fmt"
	"path/filepath"
)

// Resolver translates between absolute live paths and storage-relative paths.
type Resolver interface {
	// ToStorage translates an absolute filesystem path into a path relative
	// to the resolver's base directory.
	ToStorage(absPath string) (string, error)

	// ToLive translates a storage-relative path back into an absolute
	// filesystem path.
	ToLive(storagePath string) (string, error)

	// BaseDir returns the directory within the lnk repo where files for
	// this scope are stored.
	BaseDir() string
}

// HomeRelativeResolver maps paths relative to the user's home directory.
// This is the resolver for common and host scopes.
type HomeRelativeResolver struct {
	Home       string
	StorageDir string
}

// ToStorage returns the path of absPath relative to the home directory.
func (r *HomeRelativeResolver) ToStorage(absPath string) (string, error) {
	rel, err := filepath.Rel(r.Home, absPath)
	if err != nil {
		return "", fmt.Errorf("make home-relative: %w", err)
	}
	return rel, nil
}

// ToLive returns the absolute path for a storage-relative path.
func (r *HomeRelativeResolver) ToLive(storagePath string) (string, error) {
	return filepath.Join(r.Home, storagePath), nil
}

// BaseDir returns the configured storage directory for this scope.
func (r *HomeRelativeResolver) BaseDir() string {
	return r.StorageDir
}
