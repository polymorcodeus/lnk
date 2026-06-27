package lnkerror_test

import (
	"errors"
	"testing"

	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

func TestSentinels(t *testing.T) {
	// Verify sentinels are non-nil and distinct
	sentinels := []error{
		lnkerror.ErrManagedFilesExist,
		lnkerror.ErrGitRepoExists,
		lnkerror.ErrAlreadyManaged,
		lnkerror.ErrNotManaged,
		lnkerror.ErrNotInitialized,
		lnkerror.ErrBootstrapNotFound,
		lnkerror.ErrBootstrapFailed,
		lnkerror.ErrBootstrapPerms,
	}

	for i, err := range sentinels {
		if err == nil {
			t.Fatalf("sentinel %d is nil", i)
		}
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(err, sentinels[j]) {
				t.Fatalf("sentinel %d and %d are the same error", i, j)
			}
		}
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *lnkerror.Error
		want string
	}{
		{
			name: "sentinel only",
			err:  lnkerror.Wrap(lnkerror.ErrNotInitialized),
			want: "lnk repository not initialized",
		},
		{
			name: "with path",
			err:  lnkerror.WithPath(lnkerror.ErrNotManaged, "foo.txt"),
			want: "file is not managed by lnk: foo.txt",
		},
		{
			name: "with suggestion",
			err:  lnkerror.WithSuggestion(lnkerror.ErrBootstrapFailed, "check permissions"),
			want: "bootstrap script failed with error (check permissions)",
		},
		{
			name: "with path and suggestion",
			err:  lnkerror.WithPathAndSuggestion(lnkerror.ErrNotInitialized, "repo", "run 'lnk init'"),
			want: "lnk repository not initialized: repo (run 'lnk init')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	wrapped := lnkerror.WithPath(lnkerror.ErrNotManaged, "foo.txt")

	if !errors.Is(wrapped, lnkerror.ErrNotManaged) {
		t.Error("expected errors.Is to match the underlying sentinel")
	}
	if errors.Is(wrapped, lnkerror.ErrAlreadyManaged) {
		t.Error("expected errors.Is to NOT match a different sentinel")
	}
}
