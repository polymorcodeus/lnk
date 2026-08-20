package service_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/polymorcodeus/lnk/internal/lnkerror"
	"github.com/polymorcodeus/lnk/internal/resolver"
	"github.com/polymorcodeus/lnk/internal/testhelpers"
	"github.com/polymorcodeus/lnk/service"
)

func initProjectRepo(t *testing.T, dir string) {
	t.Helper()
	testhelpers.InitGitRepo(t, dir)
	if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com:User/Repo.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	// Create a file and commit so HEAD exists.
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# repo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds := [][]string{
		{"git", "-C", dir, "add", "."},
		{"git", "-C", dir, "commit", "-m", "init"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

func TestProjectInit_CreatesLnkInclude(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	created, err := ps.ProjectInit(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}

	manifest := filepath.Join(repoDir, ".lnkinclude")
	if !testhelpers.FileExists(t, manifest) {
		t.Error("expected .lnkinclude to be created")
	}
}

func TestProjectInit_Idempotent(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectInit(context.Background(), repoDir); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}

	created, err := ps.ProjectInit(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectInit second call: %v", err)
	}
	if created {
		t.Error("expected created=false on second call")
	}
}

func TestProjectInit_RequiresGitRepo(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	notARepo := filepath.Join(home, "not-a-repo")
	testhelpers.MakeDir(t, notARepo)

	ps := service.NewProjectService(svc)
	_, err := ps.ProjectInit(context.Background(), notARepo)
	if err == nil {
		t.Fatal("expected error outside git repo")
	}
	if !errors.Is(err, lnkerror.ErrOutsideGitRepo) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrOutsideGitRepo)
	}
}

func TestProjectInit_RequiresOrigin(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	testhelpers.InitGitRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	_, err := ps.ProjectInit(context.Background(), repoDir)
	if err == nil {
		t.Fatal("expected error without origin")
	}
	if !errors.Is(err, resolver.ErrNoOrigin) {
		t.Errorf("error = %v, want %v", err, resolver.ErrNoOrigin)
	}
}

func TestProjectAddPattern_AppendsAndNormalizes(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	absPattern := filepath.Join(repoDir, ".cursor", "rules.md")

	normalized, err := ps.ProjectAddPattern(context.Background(), repoDir, absPattern)
	if err != nil {
		t.Fatalf("ProjectAddPattern: %v", err)
	}
	if normalized != filepath.Join(".cursor", "rules.md") {
		t.Errorf("normalized = %q, want .cursor/rules.md", normalized)
	}

	manifest := filepath.Join(repoDir, ".lnkinclude")
	content, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != filepath.Join(".cursor", "rules.md")+"\n" {
		t.Errorf(".lnkinclude content = %q", string(content))
	}
}

func TestProjectAddPattern_RejectsOutsideProject(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	outside := filepath.Join(home, ".bashrc")
	_, err := ps.ProjectAddPattern(context.Background(), repoDir, outside)
	if err == nil {
		t.Fatal("expected error for pattern outside project")
	}
	if !errors.Is(err, lnkerror.ErrOutsideGitRepo) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrOutsideGitRepo)
	}
}

func TestProjectAddPattern_RejectsDuplicate(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	pattern := ".cursor/**"
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, pattern); err != nil {
		t.Fatalf("first add: %v", err)
	}

	_, err := ps.ProjectAddPattern(context.Background(), repoDir, pattern)
	if err == nil {
		t.Fatal("expected error for duplicate pattern")
	}
	if !errors.Is(err, lnkerror.ErrAlreadyManaged) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrAlreadyManaged)
	}
}

func TestProjectListPatterns_SplitsGlobalAndLocal(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	globalPath := filepath.Join(svc.RepoPath(), ".lnkinclude")
	if err := os.WriteFile(globalPath, []byte(".todo/**\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add: %v", err)
	}

	global, local, err := ps.ProjectListPatterns(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectListPatterns: %v", err)
	}
	if len(global) != 1 || global[0] != ".todo/**" {
		t.Errorf("global = %v, want [.todo/**]", global)
	}
	if len(local) != 1 || local[0] != ".cursor/**" {
		t.Errorf("local = %v, want [.cursor/**]", local)
	}
}

func TestProjectUntrackPattern_RemovesLocal(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, "agents.md"); err != nil {
		t.Fatalf("add: %v", err)
	}

	removed, isGlobal, err := ps.ProjectUntrackPattern(repoDir, "agents.md")
	if err != nil {
		t.Fatalf("ProjectUntrackPattern: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
	if isGlobal {
		t.Error("expected isGlobal=false")
	}
}

func TestProjectUntrackPattern_WarnsForGlobal(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	globalPath := filepath.Join(svc.RepoPath(), ".lnkinclude")
	if err := os.WriteFile(globalPath, []byte(".todo/**\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ps := service.NewProjectService(svc)
	removed, isGlobal, err := ps.ProjectUntrackPattern(repoDir, ".todo/**")
	if err != nil {
		t.Fatalf("ProjectUntrackPattern: %v", err)
	}
	if removed {
		t.Error("expected removed=false")
	}
	if !isGlobal {
		t.Error("expected isGlobal=true")
	}
}

func TestProjectUntrackPattern_ErrorsWhenMissing(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	_, _, err := ps.ProjectUntrackPattern(repoDir, "missing.md")
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
	if !errors.Is(err, lnkerror.ErrNotManaged) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrNotManaged)
	}
}

func TestProjectPush_MovesMatchingFileToStorage(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 1 || result.Synced[0] != ".cursor/rules.md" {
		t.Errorf("synced = %v, want [.cursor/rules.md]", result.Synced)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if !testhelpers.FileExists(t, storageFile) {
		t.Errorf("expected storage file %s", storageFile)
	}
	testhelpers.AssertSymlink(t, liveFile, storageFile)

	if logs := testhelpers.GitLog(t, svc.RepoPath()); len(logs) < 2 {
		t.Errorf("expected at least 2 commits, got %d", len(logs))
	}
}

func TestProjectPush_SkipsAlreadySymlinked(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir); err != nil {
		t.Fatalf("first push: %v", err)
	}

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}
	if len(result.Synced) != 0 {
		t.Errorf("synced = %v, want empty", result.Synced)
	}
}

func TestProjectPush_ErrorsWhenNoPatterns(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	_, err := ps.ProjectPush(context.Background(), repoDir)
	if err == nil {
		t.Fatal("expected error when no patterns")
	}
	if !errors.Is(err, lnkerror.ErrNoPatterns) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrNoPatterns)
	}
}

func TestProjectPush_SkipsGitDirectory(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, "*.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	gitConfig := filepath.Join(repoDir, ".git", "config")
	if _, err := os.Stat(gitConfig); err != nil {
		t.Fatalf("expected .git/config to exist: %v", err)
	}

	liveFile := filepath.Join(repoDir, "README.md")
	testhelpers.MakeFile(t, liveFile, "# hello\n")

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 1 || result.Synced[0] != "README.md" {
		t.Errorf("synced = %v, want [README.md]", result.Synced)
	}

	if !testhelpers.FileExists(t, gitConfig) {
		t.Error("expected .git/config to remain in place")
	}
}

func TestProjectPush_CreatesStorageDir(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, "*.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	id, err := resolver.ResolveProjectID(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("resolve project id: %v", err)
	}
	storageDir := filepath.Join(svc.RepoPath(), "projects", id)
	if testhelpers.FileExists(t, storageDir) {
		t.Fatal("expected storage dir not to exist before push")
	}

	liveFile := filepath.Join(repoDir, "notes.md")
	testhelpers.MakeFile(t, liveFile, "notes\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir); err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	if !testhelpers.FileExists(t, storageDir) {
		t.Error("expected storage dir to be created")
	}
}

func TestProjectRestore_RecreatesSymlinks(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectRestore(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectRestore: %v", err)
	}
	if len(info.Restored) != 1 {
		t.Errorf("restored = %v, want 1 entry", info.Restored)
	}
	testhelpers.AssertSymlink(t, liveFile, storageFile)
}

func TestProjectRestore_BackupsExistingFile(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	testhelpers.MakeFile(t, liveFile, "local changes\n")

	info, err := ps.ProjectRestore(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectRestore: %v", err)
	}
	if len(info.Restored) != 1 || len(info.BackedUp) != 1 {
		t.Errorf("restored = %v, backed up = %v", info.Restored, info.BackedUp)
	}

	testhelpers.AssertSymlink(t, liveFile, storageFile)
	backupFile := liveFile + ".lnk-backup"
	if !testhelpers.FileExists(t, backupFile) {
		t.Error("expected .lnk-backup file")
	}
}

func TestProjectRestore_DryRun(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir); err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectRestore(context.Background(), repoDir, true)
	if err != nil {
		t.Fatalf("ProjectRestore: %v", err)
	}
	if len(info.Restored) != 1 {
		t.Errorf("restored = %v, want 1 entry", info.Restored)
	}

	if _, err := os.Lstat(liveFile); err == nil {
		t.Error("expected symlink not to be created in dry-run mode")
	}
}

func TestProjectPull_PullsAndRestores(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	remote := testhelpers.NewBareRemote(t)
	cmds := [][]string{
		{"git", "-C", svc.RepoPath(), "remote", "add", "origin", remote},
		{"git", "-C", svc.RepoPath(), "push", "-u", "origin", "main"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	ps := service.NewProjectService(svc)
	if _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectPull(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("ProjectPull: %v", err)
	}
	if len(info.Restored) != 1 {
		t.Errorf("restored = %v, want 1 entry", info.Restored)
	}
	testhelpers.AssertSymlink(t, liveFile, storageFile)
}
