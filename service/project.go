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
	gitpkg "github.com/polymorcodeus/lnk/internal/git"
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

// ProjectInit activates project scope for the git repo containing
// projectRoot by creating an empty .lnkinclude file at the repo root if one
// does not already exist. It returns true when the file was created and
// false when it already existed.
func (ps *ProjectService) ProjectInit(ctx context.Context, projectRoot string) (bool, error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return false, err
	}

	manifest := filepath.Join(root, ".lnkinclude")
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

// ProjectAddPattern appends a pattern to the project's .lnkinclude file at
// the git root. Existing on-disk paths inside the project are normalized to
// project-relative form; anything else (globs, ! negations, files that do
// not exist yet) is stored verbatim. It returns the stored pattern and
// whether it matches at least one existing file (always true for negations,
// which are not match-checked).
func (ps *ProjectService) ProjectAddPattern(ctx context.Context, projectRoot, rawPattern string) (string, bool, error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return "", false, err
	}

	pattern, err := normalizePattern(root, rawPattern)
	if err != nil {
		return "", false, err
	}

	manifest := filepath.Join(root, ".lnkinclude")
	existing, err := patterns.Load(manifest)
	if err != nil {
		return "", false, fmt.Errorf("load .lnkinclude: %w", err)
	}
	if slices.Contains(existing, pattern) {
		return "", false, lnkerror.WithPath(lnkerror.ErrAlreadyManaged, pattern)
	}

	matched := true
	if !strings.HasPrefix(pattern, "!") {
		matched, err = matchesAnyFile(root, pattern)
		if err != nil {
			return "", false, err
		}
	}

	if err := appendPattern(manifest, pattern); err != nil {
		return "", false, err
	}

	return pattern, matched, nil
}

// ProjectListPatterns returns the effective patterns for a project, split
// into global (lnk repo root) and local (.lnkinclude at the git root) lists.
func (ps *ProjectService) ProjectListPatterns(ctx context.Context, projectRoot string) (global, local []string, err error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return nil, nil, err
	}

	global, err = patterns.Load(filepath.Join(ps.svc.RepoPath(), ".lnkinclude"))
	if err != nil {
		return nil, nil, fmt.Errorf("load global .lnkinclude: %w", err)
	}

	local, err = patterns.Load(filepath.Join(root, ".lnkinclude"))
	if err != nil {
		return nil, nil, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	return global, local, nil
}

// ProjectUntrackPattern removes a pattern from the project's .lnkinclude at
// the git root. It returns removed=true when the pattern was removed from
// the local file. When the pattern exists only in the global file, it
// returns removed=false and isGlobal=true so the caller can print a warning.
func (ps *ProjectService) ProjectUntrackPattern(ctx context.Context, projectRoot, pattern string) (removed, isGlobal bool, err error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return false, false, err
	}

	localPath := filepath.Join(root, ".lnkinclude")
	localLines, err := patterns.Load(localPath)
	if err != nil {
		return false, false, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	removed = slices.Contains(localLines, pattern)

	if removed {
		if err := rewritePatterns(localPath, localLines, pattern); err != nil {
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

// resolveProjectRoot anchors a project command at the root of the git
// working tree containing dir, and refuses the lnk repository itself:
// treating it as a project would store the repo inside its own storage.
func (ps *ProjectService) resolveProjectRoot(ctx context.Context, dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}

	root, err := gitboundary.ResolveGitRoot(ctx, abs)
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", lnkerror.WithPathAndSuggestion(lnkerror.ErrOutsideGitRepo, abs, "use 'lnk add' for host/common scope")
	}

	if ps.isLnkRepoRoot(root) {
		return "", lnkerror.WithPathAndSuggestion(lnkerror.ErrIsLnkRepository, root, "the lnk repo manages itself; project scope is for other git repositories")
	}

	return root, nil
}

// isLnkRepoRoot reports whether root is an lnk repository: either it carries
// the .lnkrepo marker (including clones of the repo elsewhere on disk) or it
// resolves to the configured repo path.
func (ps *ProjectService) isLnkRepoRoot(root string) bool {
	if _, err := os.Stat(filepath.Join(root, repoMarkerFile)); err == nil {
		return true
	}
	repoPath, err := filepath.EvalSymlinks(ps.svc.RepoPath())
	if err != nil {
		return false
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	return repoPath == canonicalRoot
}

// projectID resolves the storage identifier for the project, falling back to
// a local path-derived identifier when the repo has no origin remote.
func (ps *ProjectService) projectID(ctx context.Context, root string) (string, error) {
	id, err := resolver.ResolveProjectID(ctx, root)
	if errors.Is(err, resolver.ErrNoOrigin) {
		return resolver.LocalProjectID(root), nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve project id: %w", err)
	}
	return id, nil
}

// normalizePattern converts raw into the pattern stored in .lnkinclude. When
// raw names an existing file or directory it is relativized to the project
// root, and existing paths outside the project are rejected. Anything else
// (glob patterns, ! negations, files that do not exist yet) is returned
// verbatim.
func normalizePattern(projectRoot, raw string) (string, error) {
	pattern := strings.TrimSpace(raw)
	body := strings.TrimPrefix(pattern, "!")
	if body == "" {
		return "", lnkerror.Wrap(lnkerror.ErrEmptyPattern)
	}

	candidate := body
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(projectRoot, candidate)
	}
	if _, err := os.Lstat(candidate); err != nil {
		return pattern, nil
	}

	// Canonicalize the directory portion of both sides: git reports the repo
	// root with symlinks resolved, while the user may pass a path through a
	// symlinked prefix. The leaf is never resolved: an already-managed file
	// is a symlink into lnk storage and must still relativize to its live
	// path inside the project.
	canonicalRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonicalRoot = projectRoot
	}
	canonicalDir, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		canonicalDir = filepath.Dir(candidate)
	}
	canonicalCandidate := filepath.Join(canonicalDir, filepath.Base(candidate))

	rel, err := filepath.Rel(canonicalRoot, canonicalCandidate)
	if err != nil {
		return "", fmt.Errorf("relativize pattern %s: %w", raw, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", lnkerror.WithPathAndSuggestion(lnkerror.ErrOutsideProject, raw, "patterns must match files inside the project")
	}
	if strings.HasPrefix(pattern, "!") {
		return "!" + rel, nil
	}
	return rel, nil
}

// errPatternMatched stops the walk early once matchesAnyFile finds a hit.
var errPatternMatched = errors.New("pattern matched")

// matchesAnyFile reports whether pattern matches at least one existing file
// in the project.
func matchesAnyFile(root, pattern string) (bool, error) {
	err := walkProjectFiles(root, func(_, rel string) error {
		ok, err := patterns.Match([]string{pattern}, rel)
		if err != nil {
			return err
		}
		if ok {
			return errPatternMatched
		}
		return nil
	})
	if errors.Is(err, errPatternMatched) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// walkProjectFiles calls fn for every regular, non-symlinked file under
// root, pruning .git directories and nested git working trees. The rel path
// passed to fn uses '/' separators relative to root.
func walkProjectFiles(root string, fn func(absPath, rel string) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			if path != root && hasGitMarker(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(path, filepath.ToSlash(rel))
	})
}

// hasGitMarker reports whether dir is the root of a git working tree: a
// .git directory, or a .git file as used by submodules and linked worktrees.
func hasGitMarker(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// implicitlyExcluded reports whether rel is lnk metadata that must never be
// managed, regardless of the effective patterns.
func implicitlyExcluded(rel string) bool {
	return rel == ".lnkinclude" || strings.HasSuffix(rel, ".lnk-backup")
}

// projectTrackedFiles returns the set of slash-separated, root-relative
// paths tracked by the project's own git index.
func projectTrackedFiles(ctx context.Context, root string) (map[string]struct{}, error) {
	out, err := gitpkg.New(root).Run(ctx, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("list project-tracked files: %w\n%s", err, out)
	}
	tracked := make(map[string]struct{})
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			tracked[p] = struct{}{}
		}
	}
	return tracked, nil
}

// appendPattern appends pattern to manifest, ensuring a preceding newline
// when the file already exists and does not end with one.
func appendPattern(manifest, pattern string) error {
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
func rewritePatterns(manifest string, lines []string, dropPattern string) error {
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
	// SkippedTracked lists matched files left untouched because the
	// project's own git index tracks them (requires force to manage).
	SkippedTracked []string
}

// ProjectPush walks the project repository, moves matching files to the lnk
// project storage directory, and symlinks them back. It then stages and
// commits the changes in the lnk repo. Files tracked by the project's own
// git index are skipped unless force is set, since replacing them with
// symlinks would dirty the project's working tree with a typechange. Files
// that fail to move are collected and reported as an aggregate error after
// the rest are synced.
func (ps *ProjectService) ProjectPush(ctx context.Context, projectRoot string, force bool) (ProjectPushResult, error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return ProjectPushResult{}, err
	}

	id, err := ps.projectID(ctx, root)
	if err != nil {
		return ProjectPushResult{}, err
	}

	storageDir := filepath.Join(ps.svc.RepoPath(), "projects", id)
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return ProjectPushResult{}, fmt.Errorf("create project storage: %w", err)
	}

	global, err := patterns.Load(filepath.Join(ps.svc.RepoPath(), ".lnkinclude"))
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("load global .lnkinclude: %w", err)
	}

	local, err := patterns.Load(filepath.Join(root, ".lnkinclude"))
	if err != nil {
		return ProjectPushResult{}, fmt.Errorf("load local .lnkinclude: %w", err)
	}

	effective := slices.Concat(global, local)
	if len(effective) == 0 {
		return ProjectPushResult{}, lnkerror.Wrap(lnkerror.ErrNoPatterns)
	}

	tracked, err := projectTrackedFiles(ctx, root)
	if err != nil {
		return ProjectPushResult{}, err
	}

	result := ProjectPushResult{ProjectID: id}
	fs := &fspkg.FileSystem{}
	var failed []error

	err = walkProjectFiles(root, func(path, rel string) error {
		if implicitlyExcluded(rel) {
			return nil
		}
		match, err := patterns.Match(effective, rel)
		if err != nil {
			return err
		}
		if !match {
			return nil
		}
		if !force {
			if _, ok := tracked[rel]; ok {
				result.SkippedTracked = append(result.SkippedTracked, rel)
				return nil
			}
		}
		if err := moveToStorage(fs, path, filepath.Join(storageDir, filepath.FromSlash(rel))); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", rel, err))
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
	if hasChanges {
		if err := ps.svc.commit(ctx, "lnk: sync project "+id); err != nil {
			return result, err
		}
	}

	if len(failed) > 0 {
		return result, fmt.Errorf("%w: %w", lnkerror.ErrSyncFailed, errors.Join(failed...))
	}

	return result, nil
}

// moveToStorage moves livePath to storagePath and symlinks it back. If
// linking fails the move is rolled back so the live file is never lost.
func moveToStorage(fs *fspkg.FileSystem, livePath, storagePath string) error {
	if err := os.MkdirAll(filepath.Dir(storagePath), 0o755); err != nil {
		return fmt.Errorf("create storage directory: %w", err)
	}
	if err := fs.MoveFile(livePath, storagePath); err != nil {
		return fmt.Errorf("move to storage: %w", err)
	}
	if err := fs.CreateSymlink(storagePath, livePath); err != nil {
		if rbErr := fs.MoveFile(storagePath, livePath); rbErr != nil {
			return fmt.Errorf("create symlink: %w (rollback failed: %v)", err, rbErr)
		}
		return fmt.Errorf("create symlink: %w", err)
	}
	return nil
}

// ProjectRestore recreates symlinks for all project-scoped files from
// storage. Live files tracked by the project's own git index are left
// untouched (and reported in RestoreInfo.SkippedTracked) unless force is
// set, since replacing them with symlinks would dirty the project's working
// tree with a typechange.
func (ps *ProjectService) ProjectRestore(ctx context.Context, projectRoot string, dryRun, force bool) (RestoreInfo, error) {
	root, err := ps.resolveProjectRoot(ctx, projectRoot)
	if err != nil {
		return RestoreInfo{}, err
	}

	id, err := ps.projectID(ctx, root)
	if err != nil {
		return RestoreInfo{}, err
	}

	storageDir := filepath.Join(ps.svc.RepoPath(), "projects", id)
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		return RestoreInfo{}, lnkerror.WithPathAndSuggestion(lnkerror.ErrNotManaged, root, "run 'lnk project push' first")
	}

	tracked, err := projectTrackedFiles(ctx, root)
	if err != nil {
		return RestoreInfo{}, err
	}

	r := &scope.ProjectRootResolver{
		GitRoot:    root,
		StorageDir: storageDir,
	}

	info := RestoreInfo{}
	fs := &fspkg.FileSystem{}

	err = filepath.Walk(storageDir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(storageDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		livePath, err := r.ToLive(rel)
		if err != nil {
			return err
		}

		liveInfo, statErr := os.Lstat(livePath)
		liveExists := statErr == nil
		liveIsSymlink := liveExists && liveInfo.Mode()&os.ModeSymlink != 0

		if !force && !liveIsSymlink {
			if _, ok := tracked[rel]; ok {
				info.SkippedTracked = append(info.SkippedTracked, rel)
				return nil
			}
		}

		if liveExists {
			if liveIsSymlink {
				if !dryRun {
					if err := os.Remove(livePath); err != nil {
						return fmt.Errorf("replace symlink %s: %w", livePath, err)
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
func (ps *ProjectService) ProjectPull(ctx context.Context, projectRoot string, force bool) (RestoreInfo, error) {
	if err := ps.svc.git.Pull(ctx); err != nil {
		return RestoreInfo{}, err
	}
	return ps.ProjectRestore(ctx, projectRoot, false, force)
}
