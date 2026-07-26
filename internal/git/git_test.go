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

func newBareRemote(t *testing.T) string {
	t.Helper()
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	return remote
}

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
	t.Parallel()

	t.Run("initializes_git_repository", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()

	t.Run("configures_git_user", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T, tmp string, g *git.Git)
		wantErr bool
		errMsg  string
	}{
		{
			name: "commits_staged_changes",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
				os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
				if err := g.AddAll(); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: false,
		},
		{
			name: "fails_without_staged_changes",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
			},
			wantErr: true,
			errMsg:  "git operation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			g := initRepo(t, tmp)
			tt.setup(t, tmp, g)

			err := g.Commit("test commit")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGit_HasChanges(t *testing.T) {
	t.Parallel()

	t.Run("detects_dirty_and_clean_states", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()

	t.Run("returns_diff_for_uncommitted_changes", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()

	t.Run("stages_new_files", func(t *testing.T) {
		t.Parallel()
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
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T) (g *git.Git, cleanup func())
		wantAhead  int
		wantBehind int
		wantRemote string
		wantDirty  bool
	}{
		{
			name: "local_only_status_without_remote",
			setup: func(t *testing.T) (*git.Git, func()) {
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
				return g, func() {}
			},
			wantAhead:  1,
			wantBehind: 0,
			wantRemote: "",
			wantDirty:  false,
		},
		{
			name: "dirty_working_tree",
			setup: func(t *testing.T) (*git.Git, func()) {
				tmp := t.TempDir()
				g := initRepo(t, tmp)
				os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
				return g, func() {}
			},
			wantAhead:  0,
			wantBehind: 0,
			wantRemote: "",
			wantDirty:  true,
		},
		{
			name: "status_with_remote",
			setup: func(t *testing.T) (*git.Git, func()) {
				remote := newBareRemote(t)
				_ = pushToRemote(t, remote)

				dst := filepath.Join(t.TempDir(), "clone")
				g := git.New(dst)
				if err := g.Clone(remote); err != nil {
					t.Fatalf("Clone: %v", err)
				}
				return g, func() {}
			},
			wantAhead:  0,
			wantBehind: 0,
			wantRemote: "origin/main",
			wantDirty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, cleanup := tt.setup(t)
			defer cleanup()

			status, err := g.GetStatus()
			if err != nil {
				t.Fatal(err)
			}
			if status.Ahead != tt.wantAhead {
				t.Errorf("expected Ahead=%d, got %d", tt.wantAhead, status.Ahead)
			}
			if status.Behind != tt.wantBehind {
				t.Errorf("expected Behind=%d, got %d", tt.wantBehind, status.Behind)
			}
			if status.Remote != tt.wantRemote {
				t.Errorf("expected Remote=%q, got %q", tt.wantRemote, status.Remote)
			}
			if status.Dirty != tt.wantDirty {
				t.Errorf("expected Dirty=%v, got %v", tt.wantDirty, status.Dirty)
			}
		})
	}
}

func TestGit_Push(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (g *git.Git, remote string)
		wantErr error
	}{
		{
			name: "fails_without_remote",
			setup: func(t *testing.T) (*git.Git, string) {
				tmp := t.TempDir()
				return initRepo(t, tmp), ""
			},
			wantErr: git.ErrPush,
		},
		{
			name: "pushes_to_remote",
			setup: func(t *testing.T) (*git.Git, string) {
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
				return g, remote
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, _ := tt.setup(t)
			err := g.Push()
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Push: %v", err)
			}
		})
	}
}

func TestGit_Pull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) (g *git.Git, remote string)
		wantErr error
	}{
		{
			name: "fails_without_remote",
			setup: func(t *testing.T) (*git.Git, string) {
				tmp := t.TempDir()
				return initRepo(t, tmp), ""
			},
			wantErr: git.ErrPull,
		},
		{
			name: "pulls_from_remote",
			setup: func(t *testing.T) (*git.Git, string) {
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

				return g, remote
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, _ := tt.setup(t)
			err := g.Pull()
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Pull: %v", err)
			}
		})
	}
}

func TestGit_Clone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T) (remote, dst string)
	}{
		{
			name: "clones_from_bare_remote",
			setup: func(t *testing.T) (string, string) {
				remote := newBareRemote(t)
				_ = pushToRemote(t, remote)
				dst := filepath.Join(t.TempDir(), "clone")
				return remote, dst
			},
		},
		{
			name: "overwrites_existing_directory",
			setup: func(t *testing.T) (string, string) {
				remote := newBareRemote(t)
				_ = pushToRemote(t, remote)
				dst := filepath.Join(t.TempDir(), "clone")
				os.MkdirAll(dst, 0755)
				os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0644)
				return remote, dst
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			remote, dst := tt.setup(t)
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
			if tt.name == "overwrites_existing_directory" {
				if _, err := os.Stat(filepath.Join(dst, "old.txt")); !os.IsNotExist(err) {
					t.Error("expected old file to be removed")
				}
			}
		})
	}
}

func TestGit_Stage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, tmp string, g *git.Git)
		wantDirty bool
	}{
		{
			name: "stages_existing_file",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
				os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
				if err := g.Stage("file.txt"); err != nil {
					t.Fatalf("Stage: %v", err)
				}
			},
			wantDirty: true,
		},
		{
			name: "stages_deletion_of_removed_file",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
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
			},
			wantDirty: true,
		},
		{
			name: "ignores_untracked_missing_file",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				if err := g.Stage("nonexistent.txt"); err != nil {
					t.Fatalf("Stage: %v", err)
				}
			},
			wantDirty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			g := initRepo(t, tmp)
			tt.setup(t, tmp, g)

			dirty, err := g.HasChanges()
			if err != nil {
				t.Fatal(err)
			}
			if dirty != tt.wantDirty {
				t.Errorf("expected dirty=%v, got %v", tt.wantDirty, dirty)
			}
		})
	}
}

func TestGit_Options(t *testing.T) {
	t.Parallel()

	t.Run("WithColor_adds_color_flag", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		g := git.New(tmp, git.WithColor())
		_ = g.Init()
	})

	t.Run("WithLongTimeout_uses_long_timeout", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		g := git.New(tmp, git.WithLongTimeout())
		_ = g.Init()
	})
}

func TestGit_HasStagedChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, tmp string, g *git.Git)
		wantStaged bool
	}{
		{
			name: "detects_staged_changes",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
				os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("hello"), 0644)
				_ = g.AddAll()
			},
			wantStaged: true,
		},
		{
			name: "no_staged_changes",
			setup: func(t *testing.T, tmp string, g *git.Git) {
				configureGit(t, g)
			},
			wantStaged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			g := initRepo(t, tmp)
			tt.setup(t, tmp, g)

			staged, err := g.HasStagedChanges()
			if err != nil {
				t.Fatal(err)
			}
			if staged != tt.wantStaged {
				t.Errorf("expected staged=%v, got %v", tt.wantStaged, staged)
			}
		})
	}
}
