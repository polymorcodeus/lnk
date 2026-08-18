package service

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/polymorcodeus/lnk/internal/gitboundary"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

// ProjectAdd validates that the supplied paths are inside a git repo and
// reports that project scope is not yet implemented.
func (s *Service) ProjectAdd(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return lnkerror.Wrap(lnkerror.ErrNoPaths)
	}

	for _, input := range paths {
		absPath, err := filepath.Abs(input)
		if err != nil {
			return fmt.Errorf("resolve path %s: %w", input, err)
		}

		inside, _, err := gitboundary.IsInsideGitRepo(ctx, absPath)
		if err != nil {
			return err
		}
		if !inside {
			return lnkerror.WithPathAndSuggestion(lnkerror.ErrOutsideGitRepo, input, "use 'lnk add' for host/common scope")
		}
	}

	return lnkerror.WithSuggestion(lnkerror.ErrProjectScopeNotImplemented, "this file is inside a git repo and will be trackable once project scope lands")
}
