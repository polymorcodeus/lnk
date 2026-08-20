package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/polymorcodeus/lnk/internal/gitboundary"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
	"github.com/polymorcodeus/lnk/internal/patterns"
	"github.com/polymorcodeus/lnk/internal/resolver"
)

// ProjectService implements project-scope operations for the lnk repo
// managed by the embedded Service.
type ProjectService struct {
	svc *Service
}

// NewProjectService creates a ProjectService backed by svc.
func NewProjectService(svc *Service) *ProjectService {
	return &ProjectService{svc: svc}
}

// ProjectInit activates project scope for projectRoot by creating an empty
// ./.lnkinclude file if one does not already exist. It returns true when the
// file was created and false when it already existed.
func (ps *ProjectService) ProjectInit(ctx context.Context, projectRoot string) (bool, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return false, fmt.Errorf("resolve project root: %w", err)
	}

	if err := ps.requireInsideGitRepo(ctx, projectRoot); err != nil {
		return false, err
	}

	if _, err := resolver.ResolveProjectID(ctx, projectRoot); err != nil {
		return false, fmt.Errorf("resolve project id: %w", err)
	}

	manifest := filepath.Join(projectRoot, ".lnkinclude")
	if _, err := os.Stat(manifest); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check .lnkinclude: %w", err)
	}

	if err := os.WriteFile(manifest, []byte{}, 0o644); err != nil {
		return false, fmt.Errorf("create .lnkinclude: %w", err)
	}
	return true, nil
}

// ProjectAddPattern appends a pattern to the project's ./.lnkinclude file.
// Absolute paths inside the project are normalized to project-relative form.
// It returns the normalized pattern that was written.
func (ps *ProjectService) ProjectAddPattern(ctx context.Context, projectRoot, rawPattern string) (string, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}

	if err := ps.requireInsideGitRepo(ctx, projectRoot); err != nil {
		return "", err
	}

	if _, err := resolver.ResolveProjectID(ctx, projectRoot); err != nil {
		return "", fmt.Errorf("resolve project id: %w", err)
	}

	pattern, err := ps.normalizePattern(projectRoot, rawPattern)
	if err != nil {
		return "", err
	}

	manifest := filepath.Join(projectRoot, ".lnkinclude")
	existing, err := patterns.Load(manifest)
	if err != nil {
		return "", fmt.Errorf("load .lnkinclude: %w", err)
	}
	if slices.Contains(existing, pattern) {
		return "", lnkerror.WithPath(lnkerror.ErrAlreadyManaged, pattern)
	}

	if err := ps.appendPattern(manifest, pattern); err != nil {
		return "", err
	}

	return pattern, nil
}

// ProjectListPatterns returns the effective patterns for a project, split
// into global (lnk repo root) and local (./.lnkinclude) lists.
func (ps *ProjectService) ProjectListPatterns(ctx context.Context, projectRoot string) (global, local []string, err error) {
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve project root: %w", err)
	}

	if err := ps.requireInsideGitRepo(ctx, projectRoot); err != nil {
		return nil, nil, err
	}

	if _, err := resolver.ResolveProjectID(ctx, projectRoot); err != nil {
		return nil, nil, fmt.Errorf("resolve project id: %w", err)
	}

	global, err = patterns.Load(filepath.Join(ps.svc.RepoPath(), ".lnkinclude"))
	if err != nil {
		return nil, nil, fmt.Errorf("load global .lnkinclude: %w", err)
	}

	local, err = patterns.Load(filepath.Join(projectRoot, ".lnkinclude"))
	if err != nil {
		return nil, nil, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	return global, local, nil
}

// ProjectUntrackPattern removes a pattern from the project's ./.lnkinclude.
// It returns removed=true when the pattern was removed from the local file.
// When the pattern exists only in the global file, it returns removed=false
// and isGlobal=true so the caller can print a warning.
func (ps *ProjectService) ProjectUntrackPattern(projectRoot, pattern string) (removed, isGlobal bool, err error) {
	projectRoot, err = filepath.Abs(projectRoot)
	if err != nil {
		return false, false, fmt.Errorf("resolve project root: %w", err)
	}

	localPath := filepath.Join(projectRoot, ".lnkinclude")
	localLines, err := patterns.Load(localPath)
	if err != nil {
		return false, false, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	removed = slices.Contains(localLines, pattern)

	if removed {
		if err := ps.rewritePatterns(localPath, localLines, pattern); err != nil {
			return false, false, err
		}
		return true, false, nil
	}

	global, err := patterns.Load(filepath.Join(ps.svc.RepoPath(), ".lnkinclude"))
	if err != nil {
		return false, false, fmt.Errorf("load global .lnkinclude: %w", err)
	}
	if slices.Contains(global, pattern) {
		return false, true, nil
	}

	return false, false, lnkerror.WithPath(lnkerror.ErrNotManaged, pattern)
}

// requireInsideGitRepo returns an error if projectRoot is not inside a git repo.
func (ps *ProjectService) requireInsideGitRepo(ctx context.Context, projectRoot string) error {
	inside, _, err := gitboundary.IsInsideGitRepo(ctx, projectRoot)
	if err != nil {
		return err
	}
	if !inside {
		return lnkerror.WithPathAndSuggestion(lnkerror.ErrOutsideGitRepo, projectRoot, "use 'lnk add' for host/common scope")
	}
	return nil
}

// normalizePattern converts an absolute path inside the project to a
// project-relative pattern. Patterns that are already relative or that lie
// outside the project are returned unchanged.
func (ps *ProjectService) normalizePattern(projectRoot, rawPattern string) (string, error) {
	if !filepath.IsAbs(rawPattern) {
		absPattern, err := filepath.Abs(filepath.Join(projectRoot, rawPattern))
		if err != nil {
			return "", fmt.Errorf("resolve pattern %s: %w", rawPattern, err)
		}
		rel, err := filepath.Rel(projectRoot, absPattern)
		if err != nil {
			return "", fmt.Errorf("relativize pattern %s: %w", rawPattern, err)
		}
		if !strings.HasPrefix(rel, "..") && rel != ".." {
			return rel, nil
		}
		return rawPattern, nil
	}

	absPattern, err := filepath.Abs(rawPattern)
	if err != nil {
		return "", fmt.Errorf("resolve pattern %s: %w", rawPattern, err)
	}
	rel, err := filepath.Rel(projectRoot, absPattern)
	if err != nil {
		return "", fmt.Errorf("relativize pattern %s: %w", rawPattern, err)
	}
	if strings.HasPrefix(rel, "..") || rel == ".." {
		return "", lnkerror.WithPathAndSuggestion(lnkerror.ErrOutsideGitRepo, rawPattern, "pattern must be inside the project")
	}
	return rel, nil
}

// appendPattern appends pattern to manifest, ensuring a preceding newline when
// the file already exists and does not end with one.
func (ps *ProjectService) appendPattern(manifest, pattern string) error {
	f, err := os.OpenFile(manifest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open .lnkinclude: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	info, err := os.Stat(manifest)
	if err == nil && info.Size() > 0 {
		data, err := os.ReadFile(manifest)
		if err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return fmt.Errorf("write newline: %w", err)
			}
		}
	}

	if _, err := f.WriteString(pattern + "\n"); err != nil {
		return fmt.Errorf("write pattern: %w", err)
	}
	return nil
}

// rewritePatterns writes lines back to manifest, excluding dropPattern.
func (ps *ProjectService) rewritePatterns(manifest string, lines []string, dropPattern string) error {
	f, err := os.Create(manifest)
	if err != nil {
		return fmt.Errorf("rewrite .lnkinclude: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	w := bufio.NewWriter(f)
	for _, p := range lines {
		if p == dropPattern {
			continue
		}
		if _, err := w.WriteString(p + "\n"); err != nil {
			return fmt.Errorf("write pattern: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush .lnkinclude: %w", err)
	}
	return nil
}
