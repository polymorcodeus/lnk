package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/polymorcodeus/lnk/internal/filemanager"
	"github.com/polymorcodeus/lnk/internal/gitboundary"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

// Add tracks one or more paths in common or one host scope.
func (s *Service) Add(ctx context.Context, host string, paths []string) error {
	if err := s.requireGitRepo(); err != nil {
		return err
	}
	if len(paths) == 0 {
		return lnkerror.Wrap(lnkerror.ErrNoPaths)
	}

	var files []filemanager.FileToTrack
	host = NormalizeHost(host)
	seen := make(map[string]struct{}, len(paths))
	for _, input := range paths {
		file, err := homeRelativePath(input)
		if err != nil {
			return err
		}
		if _, ok := seen[file.RelativePath]; ok {
			return lnkerror.WithPath(lnkerror.ErrDuplicatePath, file.RelativePath)
		}
		seen[file.RelativePath] = struct{}{}

		if _, err := os.Stat(file.AbsPath); err == nil {
			inside, gitRoot, err := gitboundary.IsInsideGitRepo(ctx, file.AbsPath)
			if err != nil {
				return err
			}
			if inside {
				return lnkerror.WithPathAndSuggestion(lnkerror.ErrInsideGitRepo, file.RelativePath, fmt.Sprintf("inside git repo %s; use 'lnk project add' from within the project", gitRoot))
			}
		}

		owner, err := s.findOwner(file.RelativePath)
		if err != nil {
			return err
		}
		if owner != nil {
			return lnkerror.WithPathAndSuggestion(lnkerror.ErrAlreadyManaged, file.RelativePath, fmt.Sprintf("already managed in scope %s", owner.Host))
		}

		files = append(files, filemanager.FileToTrack{
			AbsPath:      file.AbsPath,
			RelativePath: file.RelativePath,
		})
	}

	fm, err := s.fileManager(host)
	if err != nil {
		return err
	}

	addResult, err := fm.AddMultiple(files)
	if err != nil {
		return err
	}

	if err := s.stagePaths(ctx, addResult.StagePaths...); err != nil {
		return err
	}

	pathCommit := strings.Join(addResult.StagePaths, "\n")
	if err := s.commit(ctx, fmt.Sprintf("lnk: added the following to %s\n%s", host, pathCommit)); err != nil {
		fm.RollbackAll(addResult.Rollback)
		return err
	}

	return nil
}
