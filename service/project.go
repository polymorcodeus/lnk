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

	fspkg "github.com/polymorcodeus/lnk/internal/fs"
	"github.com/polymorcodeus/lnk/internal/gitboundary"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
	"github.com/polymorcodeus/lnk/internal/patterns"
	"github.com/polymorcodeus/lnk/internal/resolver"
	"github.com/polymorcodeus/lnk/internal/scope"
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

// ProjectPushResult reports the files moved to storage and symlinked back by
// ProjectPush.
type ProjectPushResult struct {
	ProjectID string
	Synced    []string
}

// ProjectPush walks the project repository, moves matching files to the lnk
// project storage directory, and symlinks them back. It then stages and
// commits the changes in the lnk repo.
func (ps *ProjectService) ProjectPush(ctx context.Context, projectRoot string) (ProjectPushResult, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("resolve project root: %w", err)
	}

	if err := ps.requireInsideGitRepo(ctx, projectRoot); err != nil {
		return ProjectPushResult{}, err
	}

	id, err := resolver.ResolveProjectID(ctx, projectRoot)
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("resolve project id: %w", err)
	}

	storageDir := filepath.Join(ps.svc.RepoPath(), "projects", id)
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return ProjectPushResult{}, fmt.Errorf("create project storage: %w", err)
	}

	resolver := &scope.ProjectRootResolver{
		GitRoot:    projectRoot,
		StorageDir: storageDir,
	}

	global, err := patterns.Load(filepath.Join(ps.svc.RepoPath(), ".lnkinclude"))
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("load global .lnkinclude: %w", err)
	}

	local, err := patterns.Load(filepath.Join(projectRoot, ".lnkinclude"))
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	effective := append(global, local...)
	if len(effective) == 0 {
		return ProjectPushResult{}, lnkerror.Wrap(lnkerror.ErrNoPatterns)
	}

	result := ProjectPushResult{ProjectID: id}
	fs := &fspkg.FileSystem{}

	err = filepath.Walk(projectRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := resolver.ToStorage(path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		match, err := patterns.Match(effective, rel)
		if err != nil || !match {
			return nil
		}

		storagePath := filepath.Join(storageDir, rel)

		liveInfo, err := os.Lstat(path)
		if err != nil {
			return nil
		}

		if liveInfo.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(path), target)
				}
				target = filepath.Clean(target)
				if target == storagePath {
					if _, err := os.Stat(storagePath); err == nil {
						return nil
					}
				}
			}
			if err := os.Remove(path); err != nil {
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
			return nil
		}

		if moveErr := fs.MoveFile(path, storagePath); moveErr != nil {
			if _, statErr := os.Stat(storagePath); statErr == nil {
				if err := fs.CreateSymlink(storagePath, path); err != nil {
					return nil
				}
				result.Synced = append(result.Synced, rel)
			}
			return nil
		}

		if err := fs.CreateSymlink(storagePath, path); err != nil {
			_ = fs.MoveFile(storagePath, path)
			return nil
		}

		result.Synced = append(result.Synced, rel)
		return nil
	})
	if err != nil {
		return result, err
	}

	if err := ps.svc.git.AddAll(ctx); err != nil {
		return result, err
	}

	hasChanges, err := ps.svc.git.HasChanges(ctx)
	if err != nil {
		return result, err
	}
	if !hasChanges {
		return result, nil
	}

	if err := ps.svc.commit(ctx, "lnk: sync project "+id); err != nil {
		return result, err
	}

	return result, nil
}

// ProjectRestore recreates symlinks for all project-scoped files from storage.
func (ps *ProjectService) ProjectRestore(ctx context.Context, projectRoot string, dryRun bool) (RestoreInfo, error) {
	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return RestoreInfo{}, fmt.Errorf("resolve project root: %w", err)
	}

	id, err := resolver.ResolveProjectID(ctx, projectRoot)
	if err != nil {
		return RestoreInfo{}, fmt.Errorf("resolve project id: %w", err)
	}

	storageDir := filepath.Join(ps.svc.RepoPath(), "projects", id)
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		return RestoreInfo{}, lnkerror.WithPathAndSuggestion(lnkerror.ErrNotManaged, projectRoot, "run 'lnk project push' first")
	}

	resolver := &scope.ProjectRootResolver{
		GitRoot:    projectRoot,
		StorageDir: storageDir,
	}

	info := RestoreInfo{}
	fs := &fspkg.FileSystem{}

	err = filepath.Walk(storageDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil || fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(storageDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		livePath, err := resolver.ToLive(rel)
		if err != nil {
			return nil
		}

		liveInfo, err := os.Lstat(livePath)
		if err == nil {
			if liveInfo.Mode()&os.ModeSymlink != 0 {
				if !dryRun {
					if err := os.Remove(livePath); err != nil {
						return nil
					}
				}
			} else {
				info.BackedUp = append(info.BackedUp, rel)
				if !dryRun {
					backupPath := livePath + ".lnk-backup"
					if _, err := os.Lstat(backupPath); err == nil {
						return lnkerror.WithPath(lnkerror.ErrBackupExists, backupPath)
					}
					if err := os.Rename(livePath, backupPath); err != nil {
						return fmt.Errorf("backup existing file %s: %w", livePath, err)
					}
				}
			}
		}

		info.Restored = append(info.Restored, rel)
		if dryRun {
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
			return fmt.Errorf("create live parent directory: %w", err)
		}
		if err := fs.CreateSymlink(path, livePath); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return info, err
	}

	return info, nil
}

// ProjectPull pulls the lnk repo and restores project symlinks.
func (ps *ProjectService) ProjectPull(ctx context.Context, projectRoot string) (RestoreInfo, error) {
	if err := ps.svc.git.Pull(ctx); err != nil {
		return RestoreInfo{}, err
	}
	return ps.ProjectRestore(ctx, projectRoot, false)
}
