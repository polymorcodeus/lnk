// Package gitboundary detects whether a path lies inside a git working tree.
package gitboundary

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const timeout = 30 * time.Second

// ResolveGitRoot walks up from dir looking for a git working tree.
// It returns the absolute path to the repo root, or "" if none is found.
func ResolveGitRoot(ctx context.Context, dir string) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("resolve git root for %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("resolve git root for %s: not a directory", dir)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("git boundary detection timed out for %s: %w", dir, ctx.Err())
		}
		output := string(out)
		if strings.Contains(output, "not a git repository") {
			return "", nil
		}
		return "", fmt.Errorf("git boundary detection failed for %s: %w\n%s", dir, err, output)
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve absolute git root for %s: %w", root, err)
	}
	return absRoot, nil
}

// IsInsideGitRepo reports whether absPath is inside a git working tree.
// It returns the detected repo root when inside. Symlinks are not followed
// for the final path component, so a symlink located inside a repo is
// reported as inside even if its target is elsewhere.
func IsInsideGitRepo(ctx context.Context, absPath string) (bool, string, error) {
	info, err := os.Lstat(absPath)
	if err != nil {
		return false, "", fmt.Errorf("check git repo membership for %s: %w", absPath, err)
	}

	dir := absPath
	if !info.IsDir() {
		dir = filepath.Dir(absPath)
	}

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolvedDir = dir
	}

	root, err := ResolveGitRoot(ctx, resolvedDir)
	if err != nil {
		return false, "", err
	}
	if root == "" {
		return false, "", nil
	}

	var checkPath string
	if info.IsDir() {
		checkPath = resolvedDir
	} else {
		checkPath = filepath.Join(resolvedDir, filepath.Base(absPath))
	}

	rel, err := filepath.Rel(root, checkPath)
	if err != nil {
		return false, "", fmt.Errorf("relate %s to git root %s: %w", checkPath, root, err)
	}
	if strings.HasPrefix(rel, "..") || rel == "." {
		return false, "", nil
	}

	return true, root, nil
}
