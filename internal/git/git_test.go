// internal/git/git_test.go
package git_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polymorcodeus/lnk/internal/git"
)

// ---------- helpers ----------

func initRepo(t *testing.T, path string) *git.Git {
	t.Helper()
	g := git.New(path)
	if err := g.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return g
}

func configureGit(t *testing.T, g *git.Git) {
	t.Helper()
	configured := false
	if err := g.EnsureGitConfigOnce(&configured); err != nil {
		t.Fatalf("EnsureGitConfigOnce: %v", err)
	}
}

// newBareRemote creates a bare git repo to act as a remote.
func newBareRemote(t *testing.T) string {
	t.Helper()
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return remote
}

// pushToRemote sets up a source repo, commits, and pushes to remote.
func pushToRemote(t *testing.T, remote string) string {
	t.Helper()
	src := t.TempDir()
	cmds := [][]string{
		{"git", "-C", src, "init", "-b", "main"},
		{"git", "-C", src, "config", "user.email", "test@lnk"},
		{"git", "-C", src, "config", "user.name", "Lnk Test"},
		{"git", "-C", src, "remote", "add", "origin", remote},
	}
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", c, err, out)
		}
	}

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)
	pushCmds := [][]string{
		{"git", "-C", src, "add", "file.txt"},
		{"git", "-C", src, "commit", "-m", "initial"},
		{"git", "-C", src, "push", "-u", "origin", "main"},
	}
	for _, c := range pushCmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", c, err, out)
		}
	}
	return src
}

// ---------- tests ----------

func TestGit_Init(t *testing.T) {
	t.Run("initializes git repository", func(t *testing.T) {
		tmp := t.TempDir()
		g := git.New(tmp)

		if g.IsGitRepository() {
			t.Error("expected not a git repo before init")
		}

		if err := g.Init(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !g.IsGitRepository() {
			t.Error("expected git repo after init")
		}
	})
}

func TestGit_EnsureGitConfigOnce(t *testing.T) {
	t.Run("configures git user", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configured := false

		if err := g.EnsureGitConfigOnce(&configured); err != nil {
			t.Fatal(err)
		}
		if !configured {
			t.Error("expected configured to be true")
		}

		// Idempotent: second call should not error and leave flag true
		if err := g.EnsureGitConfigOnce(&configured); err != nil {
			t.Fatal(err)
		}
		if !configured {
			t.Error("expected configured to remain true")
		}
	})
}

func TestGit_Commit(t *testing.T) {
	t.Run("commits staged changes", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}

		if err := g.Commit("initial"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("fails without staged changes", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		err := g.Commit("empty")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "git operation failed") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestGit_HasChanges(t *testing.T) {
	t.Run("detects dirty and clean states", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		dirty, err := g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if dirty {
			t.Error("expected clean repo")
		}

		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		dirty, err = g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if !dirty {
			t.Error("expected dirty repo")
		}
	})
}

func TestGit_Diff(t *testing.T) {
	t.Run("returns diff for uncommitted changes", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		// Create and commit a tracked file
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("original"), 0644)
		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}
		if err := g.Commit("initial"); err != nil {
			t.Fatal(err)
		}

		// Modify the tracked file
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		diff, err := g.Diff()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diff, "hello") {
			t.Errorf("expected diff to contain content, got: %s", diff)
		}
	})
}

func TestGit_AddAll(t *testing.T) {
	t.Run("stages new files", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}

		// Staged changes are still "dirty" until committed
		dirty, err := g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if !dirty {
			t.Error("expected staged changes to show as dirty")
		}
	})
}

func TestGit_GetStatus(t *testing.T) {
	t.Run("local-only status without remote", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		// Create a commit so we have local history
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}
		if err := g.Commit("initial"); err != nil {
			t.Fatal(err)
		}

		status, err := g.GetStatus()
		if err != nil {
			t.Fatal(err)
		}
		if status.Ahead != 1 {
			t.Errorf("expected Ahead=1, got %d", status.Ahead)
		}
		if status.Behind != 0 {
			t.Errorf("expected Behind=0, got %d", status.Behind)
		}
		if status.Remote != "" {
			t.Errorf("expected no remote, got %q", status.Remote)
		}
		if status.Dirty {
			t.Error("expected clean working tree")
		}
	})

	t.Run("dirty working tree", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		status, err := g.GetStatus()
		if err != nil {
			t.Fatal(err)
		}
		if !status.Dirty {
			t.Error("expected dirty working tree")
		}
	})

	t.Run("status with remote", func(t *testing.T) {
		remote := newBareRemote(t)
		_ = pushToRemote(t, remote)

		// Clone and check status
		dst := filepath.Join(t.TempDir(), "clone")
		g := git.New(dst)
		if err := g.Clone(remote); err != nil {
			t.Fatalf("Clone: %v", err)
		}

		status, err := g.GetStatus()
		if err != nil {
			t.Fatal(err)
		}
		if status.Ahead != 0 {
			t.Errorf("expected Ahead=0 after clone, got %d", status.Ahead)
		}
		if status.Behind != 0 {
			t.Errorf("expected Behind=0 after clone, got %d", status.Behind)
		}
		if status.Dirty {
			t.Error("expected clean working tree after clone")
		}
	})
}

func TestGit_Push(t *testing.T) {
	t.Run("fails without remote", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		err := g.Push()
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, git.ErrPush) {
			t.Errorf("expected ErrPush, got %v", err)
		}
	})

	t.Run("pushes to remote", func(t *testing.T) {
		remote := newBareRemote(t)
		src := pushToRemote(t, remote)

		// Add another commit and push
		g := git.New(src)
		configureGit(t, g)
		os.WriteFile(filepath.Join(src, "file2.txt"), []byte("world"), 0644)
		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}
		if err := g.Commit("second"); err != nil {
			t.Fatal(err)
		}

		if err := g.Push(); err != nil {
			t.Fatalf("Push: %v", err)
		}
	})
}

func TestGit_Pull(t *testing.T) {
	t.Run("fails without remote", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		err := g.Pull()
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, git.ErrPull) {
			t.Errorf("expected ErrPull, got %v", err)
		}
	})

	t.Run("pulls from remote", func(t *testing.T) {
		remote := newBareRemote(t)
		src := pushToRemote(t, remote)

		// Clone into dst
		dst := filepath.Join(t.TempDir(), "clone")
		g := git.New(dst)
		if err := g.Clone(remote); err != nil {
			t.Fatalf("Clone: %v", err)
		}

		// Add commit to source and push
		gSrc := git.New(src)
		configureGit(t, gSrc)
		os.WriteFile(filepath.Join(src, "pulled.txt"), []byte("new"), 0644)
		if err := gSrc.AddAll(); err != nil {
			t.Fatal(err)
		}
		if err := gSrc.Commit("add pulled"); err != nil {
			t.Fatal(err)
		}
		if err := gSrc.Push(); err != nil {
			t.Fatal(err)
		}

		// Pull in clone
		if err := g.Pull(); err != nil {
			t.Fatalf("Pull: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dst, "pulled.txt")); err != nil {
			t.Errorf("expected pulled file to exist: %v", err)
		}
	})
}

func TestGit_Clone(t *testing.T) {
	t.Run("clones from bare remote", func(t *testing.T) {
		remote := newBareRemote(t)
		pushToRemote(t, remote)

		dst := filepath.Join(t.TempDir(), "clone")
		g := git.New(dst)

		if err := g.Clone(remote); err != nil {
			t.Fatalf("Clone: %v", err)
		}

		if !g.IsGitRepository() {
			t.Error("expected cloned directory to be a git repo")
		}
		if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
			t.Errorf("expected file.txt in clone: %v", err)
		}
	})

	t.Run("overwrites existing directory", func(t *testing.T) {
		remote := newBareRemote(t)
		pushToRemote(t, remote)

		dst := filepath.Join(t.TempDir(), "clone")
		os.MkdirAll(dst, 0755)
		os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0644)

		g := git.New(dst)
		if err := g.Clone(remote); err != nil {
			t.Fatalf("Clone: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dst, "old.txt")); !os.IsNotExist(err) {
			t.Error("expected old file to be removed")
		}
		if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
			t.Errorf("expected file.txt in clone: %v", err)
		}
	})
}

func TestGit_Stage(t *testing.T) {
	t.Run("stages existing file", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)

		if err := g.Stage("file.txt"); err != nil {
			t.Fatalf("Stage: %v", err)
		}

		dirty, err := g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if !dirty {
			t.Error("expected staged changes")
		}
	})

	t.Run("stages deletion of removed file", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)
		configureGit(t, g)

		// Create, commit, then delete
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
		if err := g.AddAll(); err != nil {
			t.Fatal(err)
		}
		if err := g.Commit("initial"); err != nil {
			t.Fatal(err)
		}

		os.Remove(filepath.Join(tmp, "file.txt"))

		if err := g.Stage("file.txt"); err != nil {
			t.Fatalf("Stage: %v", err)
		}

		dirty, err := g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if !dirty {
			t.Error("expected deletion to be staged")
		}
	})

	t.Run("ignores untracked missing file", func(t *testing.T) {
		tmp := t.TempDir()
		g := initRepo(t, tmp)

		// File never existed and was never tracked
		if err := g.Stage("nonexistent.txt"); err != nil {
			t.Fatalf("Stage: %v", err)
		}

		dirty, err := g.HasChanges()
		if err != nil {
			t.Fatal(err)
		}
		if dirty {
			t.Error("expected no changes when staging untracked missing file")
		}
	})
}
