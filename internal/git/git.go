// Package git provides Git operations for lnk.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

// Sentinel errors for git operations.
var (
	ErrGitInit     = errors.New("git init failed")
	ErrBranchSetup = errors.New("failed to set up the default branch")
	ErrGitCommand  = errors.New("git operation failed")
	ErrNoRemote    = errors.New("no remote repository is configured")
	ErrGitConfig   = errors.New("failed to configure git settings")
	ErrPush        = errors.New("failed to push changes to remote repository")
	ErrPull        = errors.New("failed to pull changes from remote repository")
	ErrGitTimeout  = errors.New("git operation timed out")
	ErrDirRemove   = errors.New("failed to prepare directory for operation")
	ErrDirCreate   = errors.New("failed to create directory")
	ErrUncommitted = errors.New("git repo has uncommitted changes")
	ErrDiff        = errors.New("failed to get diff output")
)

const (
	// shortTimeout for fast local operations (status, add, commit, etc.)
	shortTimeout = 30 * time.Second

	// longTimeout for network operations and large transfers (clone, push, pull)
	longTimeout = 5 * time.Minute
)

// Git handles Git operations
type Git struct {
	repoPath string
	timeout  time.Duration
	color    bool
}

type Option func(*Git)

// WithLongTimeout sets ctx timeout for runGitCommand
func WithLongTimeout() Option {
	return func(g *Git) { g.timeout = longTimeout }
}

// WithColor sets color.ui=always for git commmands
func WithColor() Option {
	return func(g *Git) { g.color = true }
}

func New(repoPath string, opts ...Option) *Git {
	g := &Git{
		repoPath: repoPath,
		timeout:  shortTimeout,
		color:    false,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// runGitCommand executes a git command with the given timeout.
func (g *Git) runGitCommand(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	var cmdArgs []string
	if g.color {
		cmdArgs = append(cmdArgs, "-c", "color.ui=always")
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = g.repoPath
	return cmd.CombinedOutput()
}

// Init initializes a new Git repository
func (g *Git) Init() error {
	// Try using git init -b main first (Git 2.28+)
	_, err := g.runGitCommand("init", "-b", "main")
	if err != nil {
		// Fallback to regular init + branch rename for older Git versions
		_, err := g.runGitCommand("init")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.WithSuggestion(ErrGitTimeout, "check system resources and try again")
			}
			return lnkerror.WithSuggestion(ErrGitInit, "ensure git is installed and try again")
		}

		// Set the default branch to main
		_, err = g.runGitCommand("symbolic-ref", "HEAD", "refs/heads/main")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.WithSuggestion(ErrGitTimeout, "check system resources and try again")
			}
			return lnkerror.WithSuggestion(ErrBranchSetup, "check your git installation")
		}
	}

	return nil
}

// getRemoteURL returns the URL for a remote, or error if not found
func (g *Git) getRemoteURL(name string) (string, error) {
	output, err := g.runGitCommand("remote", "get-url", name)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", lnkerror.Wrap(ErrGitTimeout)
		}
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// IsGitRepository checks if the directory contains a Git repository
func (g *Git) IsGitRepository() bool {
	gitDir := filepath.Join(g.repoPath, ".git")
	_, err := os.Stat(gitDir)
	return err == nil
}

// Stage adds a path to the index, or stages its removal if it no longer exists.
func (g *Git) Stage(path string) error {
	fullPath := filepath.Join(g.repoPath, path)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		// File was deleted — stage the removal. --ignore-unmatch avoids
		// erroring if the file was never tracked.
		_, err := g.runGitCommand("rm", "--ignore-unmatch", path)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.Wrap(ErrGitTimeout)
			}
			return lnkerror.WithSuggestion(ErrGitCommand, "check file permissions and try again")
		}
		return nil
	}

	// File exists — stage normally.
	_, err := g.runGitCommand("add", "-A", path)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrGitCommand, "check file permissions and try again")
	}
	return nil
}

// Commit creates a commit with the given message
func (g *Git) Commit(message string) error {
	_, err := g.runGitCommand("commit", "-m", message)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrGitCommand, "ensure you have staged changes and try again")
	}

	return nil
}

// EnsureGitConfigOnce sets git user.name and user.email once per Git instance.
func (g *Git) EnsureGitConfigOnce(configured *bool) error {
	if *configured {
		return nil
	}
	if err := g.ensureGitConfig(); err != nil {
		return err
	}
	*configured = true
	return nil
}

// ensureGitConfig ensures that git user.name and user.email are configured
func (g *Git) ensureGitConfig() error {
	// Check if user.name is configured
	if output, err := g.runGitCommand("config", "user.name"); err != nil || len(strings.TrimSpace(string(output))) == 0 {
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		// Set a default user.name
		_, err = g.runGitCommand("config", "user.name", "Lnk User")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.Wrap(ErrGitTimeout)
			}
			return lnkerror.WithSuggestion(ErrGitConfig, "check your git installation")
		}
	}

	// Check if user.email is configured
	if output, err := g.runGitCommand("config", "user.email"); err != nil || len(strings.TrimSpace(string(output))) == 0 {
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		// Set a default user.email
		_, err = g.runGitCommand("config", "user.email", "lnk@localhost")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.Wrap(ErrGitTimeout)
			}
			return lnkerror.WithSuggestion(ErrGitConfig, "check your git installation")
		}
	}

	return nil
}

// getRemoteInfo returns information about the default remote.
func (g *Git) getRemoteInfo() (string, error) {
	// First try to get origin remote
	url, err := g.getRemoteURL("origin")
	if err != nil {
		// If origin doesn't exist, try to get any remote
		output, err := g.runGitCommand("remote")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return "", lnkerror.Wrap(ErrGitTimeout)
			}
			return "", lnkerror.Wrap(ErrGitCommand)
		}

		remotes := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(remotes) == 0 || remotes[0] == "" {
			return "", lnkerror.WithSuggestion(ErrNoRemote, "add a remote repository first")
		}

		// Use the first remote
		url, err = g.getRemoteURL(remotes[0])
		if err != nil {
			return "", lnkerror.WithPath(ErrNoRemote, remotes[0])
		}
	}

	return url, nil
}

// StatusInfo contains repository status information.
// Output is the raw human-readable git status text.
// Parsed fields are extracted for programmatic use.
type StatusInfo struct {
	Output string // Raw git status --short (or --long) output
	Dirty  bool   // True if there are any uncommitted changes
	Ahead  int    // Commits ahead of upstream (0 if no upstream)
	Behind int    // Commits behind upstream (0 if no upstream)
	Remote string // Upstream branch name or empty if none
}

// GetStatus returns the repository status.
// Output contains the full git status text (with color if requested).
// When no remote is configured, Remote is empty and Ahead counts local commits.
func (g *Git) GetStatus() (*StatusInfo, error) {
	// Get machine-readable dirty state
	porcelain, _ := g.runGitCommand("status", "--porcelain")

	// Get human-readable output with color config
	output, err := g.runGitCommand("status")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, lnkerror.Wrap(ErrGitTimeout)
		}
		return nil, lnkerror.Wrap(ErrGitCommand)
	}

	status := &StatusInfo{
		Output: string(output),
		Dirty:  len(strings.TrimSpace(string(porcelain))) > 0,
	}

	// Check if we have a remote
	_, remoteErr := g.getRemoteInfo()
	if remoteErr != nil {
		if errors.Is(remoteErr, ErrNoRemote) {
			// Local-only repo: count all commits, mark dirty from output
			status.Ahead = g.getLocalCommitCount()
			status.Behind = 0
			status.Remote = ""
			return status, nil
		}
		return nil, remoteErr
	}

	// Get upstream branch
	upstreamOutput, err := g.runGitCommand("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, lnkerror.Wrap(ErrGitTimeout)
		}
		// No upstream set
		status.Ahead = g.getLocalCommitCount()
		status.Behind = 0
		status.Remote = "origin/main" // default assumption
		return status, nil
	}

	remoteBranch := strings.TrimSpace(string(upstreamOutput))
	status.Remote = remoteBranch
	status.Ahead = g.getAheadCount(remoteBranch)
	status.Behind = g.getBehindCount(remoteBranch)

	// Append ahead/behind summary to output for convenience
	if status.Ahead > 0 || status.Behind > 0 {
		status.Output += fmt.Sprintf("\n[remote:%s - ahead:%d behind:%d]\n", remoteBranch, status.Ahead, status.Behind)
	}

	return status, nil
}

// getLocalCommitCount returns the total number of commits on HEAD, or 0 if
// there are no commits yet (fresh repo).
func (g *Git) getLocalCommitCount() int {
	output, err := g.runGitCommand("rev-list", "--count", "HEAD")
	if err != nil {
		return 0
	}
	count := strings.TrimSpace(string(output))
	if count == "" {
		return 0
	}
	n, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}
	return n
}

// getAheadCount returns how many commits ahead of remote
func (g *Git) getAheadCount(remoteBranch string) int {
	var output []byte

	output, err := g.runGitCommand("rev-list", "--count", fmt.Sprintf("%s..HEAD", remoteBranch))
	if err != nil {
		// If remote branch doesn't exist, count all local commits
		output, err = g.runGitCommand("rev-list", "--count", "HEAD")
		if err != nil {
			return 0
		}
	}

	count := strings.TrimSpace(string(output))
	if count == "" {
		return 0
	}
	n, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}
	return n
}

// getBehindCount returns how many commits behind remote
func (g *Git) getBehindCount(remoteBranch string) int {
	output, err := g.runGitCommand("rev-list", "--count", fmt.Sprintf("HEAD..%s", remoteBranch))
	if err != nil {
		return 0
	}

	count := strings.TrimSpace(string(output))
	if count == "" {
		return 0
	}
	n, err := strconv.Atoi(count)
	if err != nil {
		return 0
	}
	return n
}

// HasChanges checks if there are uncommitted changes
func (g *Git) HasChanges() (bool, error) {
	output, err := g.runGitCommand("status", "--porcelain")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, lnkerror.Wrap(ErrGitTimeout)
		}
		return false, lnkerror.Wrap(ErrGitCommand)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// HasStagedChanges checks if there are staged changes
func (g *Git) HasStagedChanges() (bool, error) {
	output, err := g.runGitCommand("diff", "--cached", "--stat")
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// Diff returns the diff output for uncommitted changes in the repository.
// If color is true, the output will include ANSI color codes.
func (g *Git) Diff() (string, error) {

	output, err := g.runGitCommand("diff")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", lnkerror.Wrap(ErrGitTimeout)
		}
		return "", lnkerror.Wrap(ErrDiff)
	}

	return string(output), nil
}

// AddAll stages all changes in the repository
func (g *Git) AddAll() error {
	_, err := g.runGitCommand("add", "-A")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrGitCommand, "check file permissions and try again")
	}

	return nil
}

// Push pushes changes to remote
func (g *Git) Push() error {
	// First ensure we have a remote configured
	_, err := g.getRemoteInfo()
	if err != nil {
		return lnkerror.WithSuggestion(ErrPush, err.Error())
	}

	g = New(g.repoPath, WithLongTimeout())
	_, err = g.runGitCommand("push", "-u", "origin")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrPush, err.Error())
	}

	return nil
}

// Pull pulls changes from remote
func (g *Git) Pull() error {
	// First ensure we have a remote configured
	_, err := g.getRemoteInfo()
	if err != nil {
		return lnkerror.WithSuggestion(ErrPull, err.Error())
	}

	g = New(g.repoPath, WithLongTimeout())
	_, err = g.runGitCommand("pull", "origin")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrPull, err.Error())
	}

	return nil
}

// Clone clones a repository from the given URL
func (g *Git) Clone(url string) error {
	// Remove the directory if it exists to ensure clean clone
	if err := os.RemoveAll(g.repoPath); err != nil {
		return lnkerror.WithPathAndSuggestion(ErrDirRemove, g.repoPath, err.Error())
	}

	// Create parent directory
	parentDir := filepath.Dir(g.repoPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return lnkerror.WithPathAndSuggestion(ErrDirCreate, parentDir, err.Error())
	}

	// Clone the repository
	// Note: Can't use runGitCommand here because it sets cmd.Dir to g.repoPath,
	// which doesn't exist yet. Clone needs to run from parent directory.
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", url, g.repoPath)
	_, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		return lnkerror.WithSuggestion(ErrGitCommand, "check the repository URL and your network connection")
	}

	// Set up upstream tracking for main branch
	_, err = g.runGitCommand("branch", "--set-upstream-to=origin/main", "main")
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return lnkerror.Wrap(ErrGitTimeout)
		}
		// If main doesn't exist, try master
		_, err = g.runGitCommand("branch", "--set-upstream-to=origin/master", "master")
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return lnkerror.Wrap(ErrGitTimeout)
			}
			// If that also fails, try to set upstream for current branch
			_, err = g.runGitCommand("branch", "--set-upstream-to=origin/HEAD")
			if err != nil {
				return err
			}
		}
	}

	return nil
}
