// Package service implements the v2 lnk command semantics.
package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	filemgr "github.com/polymorcodeus/lnk/internal/filemanager"
	fspkg "github.com/polymorcodeus/lnk/internal/fs"
	gitpkg "github.com/polymorcodeus/lnk/internal/git"
	"github.com/polymorcodeus/lnk/internal/tracker"
)

const (
	repoMarkerFile    = ".lnkrepo"
	repoMarkerVersion = "version=2\n"
	repoMarkerLegacy  = "version=1\n"
	scopeCommon       = "common"
)

// Service owns the v2 CLI semantics while reusing the existing low-level git
// and filesystem collaborators.
type Service struct {
	repoPath      string
	git           *gitpkg.Git
	format        tracker.RepoFormat
	gitConfigured bool
}

type Option func(*Service)

// WithColor is a convenience wrapper that returns git.WithColor
func WithColor(enabled bool) Option {
	if enabled {
		return WithGitOptions(gitpkg.WithColor())
	}
	return WithGitOptions() // explicit no-color
}

// WithGitOptions creates new git instance with git.Options
func WithGitOptions(opts ...gitpkg.Option) Option {
	return func(s *Service) {
		s.git = gitpkg.New(s.repoPath, opts...)
	}
}

// ScopeList describes tracked items for one storage scope.
type ScopeList struct {
	Name  string
	Items []string
}

// ListResult contains tracked items grouped by storage scope.
type ListResult struct {
	Scopes []ScopeList
}

// RestoreInfo reports machine-state changes made or planned by restore/update.
type RestoreInfo struct {
	Restored []string
	BackedUp []string
}

// OwnershipCollision describes a path claimed by more than one scope.
type OwnershipCollision struct {
	Path   string
	Scopes []string
}

type profileItem struct {
	RelativePath string
	RepoPath     string
	LivePath     string
}

type owner struct {
	Host string
}

// New returns a Service with sensible defaults.
func New(repoPath string) *Service {
	return NewBuilder(repoPath)
}

// NewBuilder returns a Service with custom options.
// Git is always configured; pass git.WithColor() or other git options
// via service.WithGitOptions(...) if needed.
func NewBuilder(repoPath string, opts ...Option) *Service {
	s := &Service{
		repoPath: repoPath,
		git:      gitpkg.New(repoPath),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ResolveRepoPath resolves the repo path from explicit flag or environment.
func ResolveRepoPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	// Load from ENV vars
	if env := cmp.Or(os.Getenv("LNK_REPO"), os.Getenv("LNK_HOME")); env != "" {
		return env
	}
	// Construct from XDG_CONFIG or make one in HomeDir
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", "lnk")
		}
		xdgConfig = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(xdgConfig, "lnk")
}

// RepoPath returns the configured repo path.
func (s *Service) RepoPath() string {
	return s.repoPath
}

// NormalizeHost returns "common" for an empty host string, otherwise returns the host unchanged.
func NormalizeHost(host string) string {
	if host == "" {
		return scopeCommon
	}
	return host
}

// commit is a thin wrapper that ensures git config is set once before committing.
func (s *Service) commit(message string) error {
	if err := s.git.EnsureGitConfigOnce(&s.gitConfigured); err != nil {
		return err
	}
	return s.git.Commit(message)
}

// requireGitRepo returns an error if the configured path is not a git repository.
func (s *Service) requireGitRepo() error {
	if !s.git.IsGitRepository() {
		return fmt.Errorf("lnk repository not initialized: run 'lnk init' first")
	}
	return nil
}

// fileManager builds a filemanager.Manager for the given host scope.
func (s *Service) fileManager(host string) (*filemgr.Manager, error) {
	format, err := s.getFormat()
	if err != nil {
		return nil, err
	}
	fs := fspkg.New()
	tr := tracker.New(s.repoPath, host, format)
	return filemgr.New(s.repoPath, host, fs, tr), nil
}

// hosts returns all host scope names found in the repo, with "common" first.
func (s *Service) hosts() ([]string, error) {
	entries, err := os.ReadDir(s.repoPath)
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if after, ok := strings.CutPrefix(name, ".lnk."); ok && after != "common" {
			hosts = append(hosts, after)
		}
	}
	slices.Sort(hosts)

	// prepend common host - covers both v1/v2 formatting
	// used as first lookup target within scopes
	return slices.Insert(hosts, 0, "common"), nil
}

// findOwner returns the first scope that manages the given relative path, or nil if none.
func (s *Service) findOwner(relativePath string) (*owner, error) {
	hosts, err := s.hosts()
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		owner, err := s.findOwnerInScope(relativePath, host)
		if err != nil {
			return nil, err
		}
		if owner != nil {
			return owner, nil
		}
	}
	return nil, nil
}

// findOwnerInScope checks whether a specific scope manages the given relative path.
func (s *Service) findOwnerInScope(relativePath, host string) (*owner, error) {
	format, err := s.getFormat()
	if err != nil {
		return nil, err
	}
	host = NormalizeHost(host)
	items, err := tracker.New(s.repoPath, host, format).GetManagedItems()
	if err != nil {
		return nil, err
	}
	if slices.Contains(items, relativePath) {
		return &owner{Host: host}, nil
	}
	return nil, nil
}

// resolveRemovalScope determines which scope owns a path for removal/forget operations.
func (s *Service) resolveRemovalScope(input, host string) (string, filemgr.FileToTrack, error) {
	// host is explicitly not normalized to allow for input lookup across all scopes
	file, err := homeRelativePath(input)
	if err != nil {
		return "", filemgr.FileToTrack{}, err
	}
	if host != "" {
		owner, ownerErr := s.findOwnerInScope(file.RelativePath, host)
		if ownerErr != nil {
			return "", filemgr.FileToTrack{}, ownerErr
		}
		if owner == nil {
			return "", filemgr.FileToTrack{}, fmt.Errorf("path is not managed in scope %s: %s", host, file.RelativePath)
		}
		return host, file, nil
	}

	owner, ownerErr := s.findOwner(file.RelativePath)
	if ownerErr != nil {
		return "", filemgr.FileToTrack{}, ownerErr
	}
	if owner == nil {
		return "", filemgr.FileToTrack{}, fmt.Errorf("path is not managed: %s", file.RelativePath)
	}
	if owner.Host != scopeCommon {
		return "", filemgr.FileToTrack{}, fmt.Errorf("path is managed in host scope %s; use --host %s", owner.Host, owner.Host)
	}
	return scopeCommon, file, nil
}

// scanCollisions finds paths that are tracked in more than one scope.
func (s *Service) scanCollisions() ([]OwnershipCollision, error) {
	format, err := s.getFormat()
	if err != nil {
		return nil, err
	}
	paths := make(map[string][]string)
	hosts, err := s.hosts()
	if err != nil {
		return nil, err
	}
	for _, host := range hosts {
		items, itemErr := tracker.New(s.repoPath, host, format).GetManagedItems()
		if itemErr != nil {
			return nil, itemErr
		}
		for _, item := range items {
			paths[item] = append(paths[item], host)
		}
	}
	collisions := make([]OwnershipCollision, 0)
	for path, scopes := range paths {
		if len(scopes) > 1 {
			slices.Sort(scopes)
			collisions = append(collisions, OwnershipCollision{Path: path, Scopes: scopes})
		}
	}
	slices.SortFunc(collisions, func(a, b OwnershipCollision) int {
		return strings.Compare(a.Path, b.Path)
	})
	return collisions, nil
}

// repointManagedSymlink recreates the live symlink so it points to the new storage path.
func (s *Service) repointManagedSymlink(relativePath, targetPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	livePath := filepath.Join(homeDir, relativePath)
	if _, err := os.Lstat(livePath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	managedByV1 := isManagedSymlink(livePath, filepath.Clean(filepath.Join(s.repoPath, relativePath)))
	managedByV2 := isManagedSymlink(livePath, filepath.Clean(filepath.Join(s.repoPath, filepath.Base(filepath.Dir(targetPath)))))
	if !managedByV1 && !managedByV2 {
		if info, err := os.Lstat(livePath); err == nil && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
	}
	if err := os.Remove(livePath); err != nil {
		return fmt.Errorf("remove existing symlink: %w", err)
	}
	return fspkg.New().CreateSymlink(targetPath, livePath)
}

// writeMarkerFile writes the version marker to disk.
func (s *Service) writeMarkerFile(ver string) error {
	return os.WriteFile(filepath.Join(s.repoPath, repoMarkerFile), []byte(ver), 0o644)
}

// stagePaths stages the given paths in the repo via internal git.Stage.
func (s *Service) stagePaths(paths ...string) error {
	for _, path := range paths {
		if err := s.git.Stage(path); err != nil {
			return err
		}
	}
	return nil
}

// execGit runs a git command inside the repo directory.
func (s *Service) execGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// homeRelativePath resolves an input path to an absolute path and its relative path from $HOME.
func homeRelativePath(input string) (filemgr.FileToTrack, error) {
	absPath, err := filepath.Abs(input)
	if err != nil {
		return filemgr.FileToTrack{}, fmt.Errorf("resolve path %s: %w", input, err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filemgr.FileToTrack{}, fmt.Errorf("resolve home directory: %w", err)
	}
	relativePath, err := filepath.Rel(homeDir, absPath)
	if err != nil {
		return filemgr.FileToTrack{}, fmt.Errorf("resolve relative path %s: %w", input, err)
	}
	cleaned := filepath.Clean(relativePath)
	if cleaned == "." || strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return filemgr.FileToTrack{}, fmt.Errorf("path must be inside $HOME: %s", input)
	}
	file := filemgr.FileToTrack{
		RelativePath: cleaned,
		AbsPath:      absPath,
	}
	return file, nil
}

// isManagedSymlink checks whether the live path is a symlink pointing to the expected repo target.
func isManagedSymlink(livePath, expectedTarget string) bool {
	info, err := os.Lstat(livePath)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(livePath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(livePath), target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	expectedAbs, err := filepath.Abs(expectedTarget)
	if err != nil {
		return false
	}
	return targetAbs == expectedAbs
}

// profileItems returns the ordered list of items that make up the effective machine profile.
func (s *Service) profileItems(host string) ([]profileItem, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}

	format, err := s.getFormat()
	if err != nil {
		return nil, err
	}

	// scopes should always include common, additional host added to scope
	scopes := []string{"common"}
	if host != "common" {
		scopes = append(scopes, host)
	}

	items := make([]profileItem, 0)
	seen := make(map[string]struct{})
	for _, scope := range scopes {
		tr := tracker.New(s.repoPath, scope, format)
		managedItems, trackErr := tr.GetManagedItems()
		if trackErr != nil {
			return nil, trackErr
		}
		for _, relativePath := range managedItems {
			if _, ok := seen[relativePath]; ok {
				return nil, fmt.Errorf("profile contains duplicate ownership for %s", relativePath)
			}
			seen[relativePath] = struct{}{}
			hostPath, err := tr.HostStoragePath()
			if err != nil {
				return nil, err
			}
			items = append(items, profileItem{
				RelativePath: relativePath,
				RepoPath:     filepath.Join(hostPath, relativePath),
				LivePath:     filepath.Join(homeDir, relativePath),
			})
		}
	}

	slices.SortFunc(items, func(a, b profileItem) int {
		return strings.Compare(a.RelativePath, b.RelativePath)
	})
	return items, nil
}

// hasLnkMarker reports whether the version marker file exists in the repo.
func (s *Service) hasLnkMarker() bool {
	markerPath := filepath.Join(s.repoPath, repoMarkerFile)
	if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

// IsLnkRepository checks if the repository appears to be managed by lnk
func (s *Service) IsLnkRepository() bool {
	if !s.git.IsGitRepository() {
		return false
	}

	return s.hasLnkMarker()
}
