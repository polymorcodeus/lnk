// Package resolver maps a directory to a stable project identifier derived
// from its git origin remote.
package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const timeout = 30 * time.Second

// ErrNoOrigin is returned when a git repo has no origin remote.
var ErrNoOrigin = errors.New("no origin remote configured")

// ResolveProjectID returns a normalized identifier for the git repo
// containing dir. The identifier is the normalized URL of the origin remote.
func ResolveProjectID(ctx context.Context, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("resolve project id for %s: %w", dir, ctx.Err())
		}
		output := string(out)
		if strings.Contains(output, "No such remote") {
			return "", ErrNoOrigin
		}
		return "", fmt.Errorf("resolve project id for %s: %w\n%s", dir, err, output)
	}

	return NormalizeRemoteURL(strings.TrimSpace(string(out))), nil
}

// NormalizeRemoteURL strips auth, scheme, and port information and returns a
// lowercase host/path identifier.
func NormalizeRemoteURL(u string) string {
	if strings.Contains(u, "://") {
		parsed, err := url.Parse(u)
		if err == nil && parsed.Host != "" {
			u = path.Join(parsed.Hostname(), parsed.Path)
		}
	} else {
		// SSH shorthand: [user@]host:path
		u = strings.Replace(u, ":", "/", 1)
		if idx := strings.Index(u, "@"); idx >= 0 {
			u = u[idx+1:]
		}
	}

	u = strings.TrimSuffix(u, ".git")
	u = strings.Trim(u, "/")
	return strings.ToLower(u)
}

// LocalProjectID returns a deterministic identifier for a repository without
// an origin remote, derived from the canonical path of the repo root. Local
// IDs are machine-specific by design: two machines checking out the same
// local-only repo to different paths produce different IDs.
func LocalProjectID(root string) string {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		canonical = root
	}
	sum := sha256.Sum256([]byte(canonical))
	base := strings.ToLower(filepath.Base(canonical))
	return fmt.Sprintf("local/%s-%s", base, hex.EncodeToString(sum[:])[:8])
}
