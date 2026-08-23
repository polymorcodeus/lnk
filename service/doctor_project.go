package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/polymorcodeus/lnk/internal/patterns"
	"github.com/polymorcodeus/lnk/internal/scope"
)

// scanProjectIssues runs project-scope health checks and returns any findings.
func (s *Service) scanProjectIssues(ctx context.Context) ([]ProjectIssue, error) {
	var issues []ProjectIssue

	orphaned, err := s.findOrphanedProjectStorage(ctx)
	if err != nil {
		return nil, fmt.Errorf("find orphaned project storage: %w", err)
	}
	issues = append(issues, orphaned...)

	broken, err := s.findBrokenProjectSymlinks(ctx)
	if err != nil {
		return nil, fmt.Errorf("find broken project symlinks: %w", err)
	}
	issues = append(issues, broken...)

	emptyPatterns, err := s.findEmptyProjectPatterns(ctx)
	if err != nil {
		return nil, fmt.Errorf("find empty project patterns: %w", err)
	}
	issues = append(issues, emptyPatterns...)

	return issues, nil
}

// findOrphanedProjectStorage returns issues for stored projects whose ID does
// not match any live project directory (identified by a .lnkinclude file).
func (s *Service) findOrphanedProjectStorage(ctx context.Context) ([]ProjectIssue, error) {
	stored, err := s.storedProjectIDs()
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, nil
	}

	home, err := s.homeDir()
	if err != nil {
		return nil, err
	}

	liveIDs := make(map[string]struct{})
	err = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || info.Name() == ".git" {
			return nil
		}

		manifest := filepath.Join(path, ".lnkinclude")
		if _, statErr := os.Stat(manifest); errors.Is(statErr, os.ErrNotExist) {
			return nil
		} else if statErr != nil {
			return statErr
		}

		id, err := resolveProjectID(ctx, path)
		if err != nil {
			return nil
		}
		liveIDs[id] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var issues []ProjectIssue
	for _, id := range stored {
		if _, ok := liveIDs[id]; ok {
			continue
		}
		issues = append(issues, ProjectIssue{
			ProjectID: id,
			Issue:     "orphaned project storage with no corresponding repo on disk",
			Severity:  "warning",
			Suggestion: fmt.Sprintf(
				"run 'rm -rf %s' or verify the project is still needed",
				filepath.Join(s.repoPath, "projects", id)),
		})
	}

	slices.SortFunc(issues, func(a, b ProjectIssue) int {
		return strings.Compare(a.ProjectID, b.ProjectID)
	})
	return issues, nil
}

// storedProjectIDs returns the IDs of all projects stored under projects/.
func (s *Service) storedProjectIDs() ([]string, error) {
	root := filepath.Join(s.repoPath, "projects")
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{})
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != projectMarkerFile {
			return nil
		}
		id, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		ids[filepath.ToSlash(id)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	slices.Sort(result)
	return result, nil
}

// findBrokenProjectSymlinks walks live project directories and reports
// project-scope symlinks whose storage target no longer exists.
func (s *Service) findBrokenProjectSymlinks(ctx context.Context) ([]ProjectIssue, error) {
	home, err := s.homeDir()
	if err != nil {
		return nil, err
	}

	var issues []ProjectIssue
	err = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || info.Name() == ".git" {
			return nil
		}

		manifest := filepath.Join(path, ".lnkinclude")
		if _, statErr := os.Stat(manifest); errors.Is(statErr, os.ErrNotExist) {
			return nil
		} else if statErr != nil {
			return statErr
		}

		id, err := resolveProjectID(ctx, path)
		if err != nil {
			return nil
		}
		storageDir := filepath.Join(s.repoPath, "projects", id)

		_ = filepath.Walk(path, func(livePath string, liveInfo os.FileInfo, err error) error {
			if err != nil || liveInfo.IsDir() {
				return nil
			}
			if liveInfo.Mode()&os.ModeSymlink == 0 {
				return nil
			}
			if !isStorageSymlink(livePath, storageDir) {
				return nil
			}

			target, err := os.Readlink(livePath)
			if err != nil {
				return nil
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(livePath), target)
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				return nil
			}

			rel, err := filepath.Rel(path, livePath)
			if err != nil {
				return nil
			}

			issues = append(issues, ProjectIssue{
				ProjectID:  id,
				Issue:      fmt.Sprintf("broken symlink: %s", filepath.ToSlash(rel)),
				Severity:   "error",
				Suggestion: "run 'lnk project restore' from the project directory",
			})
			return nil
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(issues, func(a, b ProjectIssue) int {
		if cmp := strings.Compare(a.ProjectID, b.ProjectID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Issue, b.Issue)
	})
	return issues, nil
}

// findEmptyProjectPatterns reports .lnkinclude patterns that match no files in
// the project directory.
func (s *Service) findEmptyProjectPatterns(ctx context.Context) ([]ProjectIssue, error) {
	home, err := s.homeDir()
	if err != nil {
		return nil, err
	}

	var issues []ProjectIssue
	err = filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || info.Name() == ".git" {
			return nil
		}

		manifest := filepath.Join(path, ".lnkinclude")
		if _, statErr := os.Stat(manifest); errors.Is(statErr, os.ErrNotExist) {
			return nil
		} else if statErr != nil {
			return statErr
		}

		id, err := resolveProjectID(ctx, path)
		if err != nil {
			return nil
		}

		global, _ := patterns.Load(filepath.Join(s.repoPath, ".lnkinclude"))
		local, err := patterns.Load(manifest)
		if err != nil {
			return nil
		}
		allPatterns := append(global, local...)

		patternMatches := make(map[string]int)
		for _, p := range allPatterns {
			if p == "" || strings.HasPrefix(p, "#") || strings.HasPrefix(p, "!") {
				continue
			}
			patternMatches[p] = 0
		}
		if len(patternMatches) == 0 {
			return nil
		}

		_ = filepath.Walk(path, func(filePath string, fileInfo os.FileInfo, err error) error {
			if err != nil || fileInfo.IsDir() {
				return nil
			}

			rel, err := filepath.Rel(path, filePath)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if implicitlyExcluded(rel) {
				return nil
			}

			for _, p := range allPatterns {
				match, err := patterns.Match([]string{p}, rel)
				if err != nil {
					return nil
				}
				if match {
					patternMatches[p]++
				}
			}
			return nil
		})

		for pattern, count := range patternMatches {
			if count > 0 {
				continue
			}
			issues = append(issues, ProjectIssue{
				ProjectID: id,
				Issue:     fmt.Sprintf("pattern matches no files: %q", pattern),
				Severity:  "warning",
				Suggestion: fmt.Sprintf(
					"check the pattern in %s or remove it if no longer needed",
					manifest),
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(issues, func(a, b ProjectIssue) int {
		if cmp := strings.Compare(a.ProjectID, b.ProjectID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Issue, b.Issue)
	})
	return issues, nil
}

// homeDir returns the user's home directory. It prefers the home directory
// embedded in the service's resolver when available.
func (s *Service) homeDir() (string, error) {
	if r, ok := s.resolver.(*scope.HomeRelativeResolver); ok {
		return r.Home, nil
	}
	return os.UserHomeDir()
}
