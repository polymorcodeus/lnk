// Package lnkerror provides a single error wrapper type and sentinel errors for the lnk application.
package lnkerror

import "errors"

// Sentinel errors for lnk operations.
var (
	ErrGitRepoExists              = errors.New("directory contains an existing Git repository")
	ErrAlreadyManaged             = errors.New("file is already managed by lnk")
	ErrNotManaged                 = errors.New("file is not managed by lnk")
	ErrNotInitialized             = errors.New("lnk repository not initialized")
	ErrNoPaths                    = errors.New("no paths provided")
	ErrDuplicatePath              = errors.New("duplicate path in one add invocation")
	ErrNotInHome                  = errors.New("path must be inside $HOME")
	ErrNoChanges                  = errors.New("no changes to commit")
	ErrDirtyTree                  = errors.New("working tree is dirty")
	ErrDuplicateOwnership         = errors.New("profile contains duplicate ownership")
	ErrInvalidFlags               = errors.New("flags cannot be combined")
	ErrBackupExists               = errors.New("backup path already exists")
	ErrBootstrapFailed            = errors.New("bootstrap script failed with error")
	ErrBootstrapPerms             = errors.New("failed to make bootstrap script executable")
	ErrInsideGitRepo              = errors.New("file is inside a git repo")
	ErrOutsideGitRepo             = errors.New("file is outside any git repo")
	ErrProjectScopeNotImplemented = errors.New("project scope is not yet implemented")
	ErrNoPatterns                 = errors.New("no patterns defined")
	ErrIsLnkRepository            = errors.New("directory is the lnk repository")
	ErrOutsideProject             = errors.New("path is outside the project")
	ErrEmptyPattern               = errors.New("pattern is empty")
	ErrSyncFailed                 = errors.New("some files failed to sync")
	ErrForeignHook                = errors.New("existing hook not managed by lnk")
	ErrPathExists                 = errors.New("path already exists")
	ErrMixedCreateTypes           = errors.New("cannot mix files and directories in one invocation")
)

// Error wraps a sentinel error with optional context for display.
// This is the only custom error type in the codebase.
type Error struct {
	Err        error  // Underlying sentinel error
	Path       string // Optional path for display
	Suggestion string // Optional suggestion for user
}

// Error implements the error interface, appending path and suggestion when present.
func (e *Error) Error() string {
	msg := e.Err.Error()
	if e.Path != "" {
		msg += ": " + e.Path
	}
	if e.Suggestion != "" {
		msg += " (" + e.Suggestion + ")"
	}
	return msg
}

// Unwrap returns the underlying sentinel error.
func (e *Error) Unwrap() error {
	return e.Err
}

// Wrap creates an Error with just the sentinel.
func Wrap(err error) *Error {
	return &Error{Err: err}
}

// WithPath creates an Error with path context.
func WithPath(err error, path string) *Error {
	return &Error{Err: err, Path: path}
}

// WithSuggestion creates an Error with a suggestion.
func WithSuggestion(err error, suggestion string) *Error {
	return &Error{Err: err, Suggestion: suggestion}
}

// WithPathAndSuggestion creates an Error with both.
func WithPathAndSuggestion(err error, path, suggestion string) *Error {
	return &Error{Err: err, Path: path, Suggestion: suggestion}
}
