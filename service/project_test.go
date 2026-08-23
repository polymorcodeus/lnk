package service_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestProjectInit_FromSubdirAnchorsAtGitRoot(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	subDir := filepath.Join(repoDir, "sub", "dir")
	testhelpers.MakeDir(t, subDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	created, err := ps.ProjectInit(context.Background(), subDir)
	if err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	if !created {
		t.Error("expected created=true")
	}

	if !testhelpers.FileExists(t, filepath.Join(repoDir, ".lnkinclude")) {
		t.Error("expected .lnkinclude at the git root")
	}
	if testhelpers.FileExists(t, filepath.Join(subDir, ".lnkinclude")) {
		t.Error("expected no .lnkinclude in the subdirectory")
	}
}

func TestProjectCommands_RefuseLnkRepository(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	cloneDir := filepath.Join(home, "elsewhere", "dotfiles-clone")

	// A copy of the lnk repo at a different path still carries the marker.
	testhelpers.MakeDir(t, cloneDir)
	testhelpers.InitGitRepo(t, cloneDir)
	if err := os.WriteFile(filepath.Join(cloneDir, ".lnkrepo"), []byte("version=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{svc.RepoPath(), cloneDir} {
		ps := service.NewProjectService(svc)
		if _, err := ps.ProjectInit(context.Background(), dir); !errors.Is(err, lnkerror.ErrIsLnkRepository) {
			t.Errorf("ProjectInit(%s) error = %v, want %v", dir, err, lnkerror.ErrIsLnkRepository)
		}
		if _, err := ps.ProjectPush(context.Background(), dir, false); !errors.Is(err, lnkerror.ErrIsLnkRepository) {
			t.Errorf("ProjectPush(%s) error = %v, want %v", dir, err, lnkerror.ErrIsLnkRepository)
		}
		if _, _, err := ps.ProjectListPatterns(context.Background(), dir); !errors.Is(err, lnkerror.ErrIsLnkRepository) {
			t.Errorf("ProjectListPatterns(%s) error = %v, want %v", dir, err, lnkerror.ErrIsLnkRepository)
		}
		if _, _, err := ps.ProjectUntrackPattern(context.Background(), dir, "x"); !errors.Is(err, lnkerror.ErrIsLnkRepository) {
			t.Errorf("ProjectUntrackPattern(%s) error = %v, want %v", dir, err, lnkerror.ErrIsLnkRepository)
		}
		if _, err := ps.ProjectRestore(context.Background(), dir, false, false); !errors.Is(err, lnkerror.ErrIsLnkRepository) {
			t.Errorf("ProjectRestore(%s) error = %v, want %v", dir, err, lnkerror.ErrIsLnkRepository)
		}
	}
}

func TestProjectAddPattern_AppendsAndNormalizes(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	ps := service.NewProjectService(svc)
	normalized, matched, err := ps.ProjectAddPattern(context.Background(), repoDir, liveFile)
	if err != nil {
		t.Fatalf("ProjectAddPattern: %v", err)
	}
	if normalized != filepath.Join(".cursor", "rules.md") {
		t.Errorf("normalized = %q, want .cursor/rules.md", normalized)
	}
	if !matched {
		t.Error("expected matched=true for an existing file")
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

func TestProjectAddPattern_StoresPatternsVerbatim(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)

	// A ! negation is not a path and must be stored as written.
	negation, matched, err := ps.ProjectAddPattern(context.Background(), repoDir, "!AGENTS.md")
	if err != nil {
		t.Fatalf("add negation: %v", err)
	}
	if negation != "!AGENTS.md" {
		t.Errorf("negation = %q, want !AGENTS.md", negation)
	}
	if !matched {
		t.Error("expected matched=true for negations (match check is skipped)")
	}

	// A glob for a directory that does not exist yet keeps its trailing slash.
	glob, matched, err := ps.ProjectAddPattern(context.Background(), repoDir, ".todo/")
	if err != nil {
		t.Fatalf("add glob: %v", err)
	}
	if glob != ".todo/" {
		t.Errorf("glob = %q, want .todo/", glob)
	}
	if matched {
		t.Error("expected matched=false for a pattern with no current files")
	}

	manifest := filepath.Join(repoDir, ".lnkinclude")
	content, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "!AGENTS.md\n.todo/\n" {
		t.Errorf(".lnkinclude content = %q", string(content))
	}
}

func TestProjectAddPattern_RelativizesExistingDir(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	testhelpers.MakeFile(t, filepath.Join(repoDir, ".todo", "a.md"), "a\n")

	ps := service.NewProjectService(svc)
	normalized, matched, err := ps.ProjectAddPattern(context.Background(), repoDir, ".todo/")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if normalized != ".todo" {
		t.Errorf("normalized = %q, want .todo", normalized)
	}
	if !matched {
		t.Error("expected matched=true for a dir with files")
	}
}

func TestProjectAddPattern_NegatesManagedSymlink(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "AGENTS.md"); err != nil {
		t.Fatalf("add include: %v", err)
	}
	testhelpers.MakeFile(t, filepath.Join(repoDir, "AGENTS.md"), "agents\n")
	if _, err := ps.ProjectPush(context.Background(), repoDir, false); err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	// AGENTS.md is now a symlink into lnk storage (outside the project).
	// Negating it must still relativize to its live path.
	negation, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "!AGENTS.md")
	if err != nil {
		t.Fatalf("add negation: %v", err)
	}
	if negation != "!AGENTS.md" {
		t.Errorf("negation = %q, want !AGENTS.md", negation)
	}
}

func TestProjectAddPattern_DotDotFilenameInsideProject(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	testhelpers.MakeFile(t, filepath.Join(repoDir, "..foo"), "odd name\n")

	ps := service.NewProjectService(svc)
	normalized, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "..foo")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if normalized != "..foo" {
		t.Errorf("normalized = %q, want ..foo", normalized)
	}
}

func TestProjectAddPattern_RejectsOutsideProject(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	outside := filepath.Join(home, ".bashrc")
	testhelpers.MakeFile(t, outside, "# bashrc\n")

	ps := service.NewProjectService(svc)
	_, _, err := ps.ProjectAddPattern(context.Background(), repoDir, outside)
	if err == nil {
		t.Fatal("expected error for pattern outside project")
	}
	if !errors.Is(err, lnkerror.ErrOutsideProject) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrOutsideProject)
	}
}

func TestProjectAddPattern_RejectsEmpty(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	for _, pattern := range []string{"", "   ", "!"} {
		if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, pattern); !errors.Is(err, lnkerror.ErrEmptyPattern) {
			t.Errorf("add %q error = %v, want %v", pattern, err, lnkerror.ErrEmptyPattern)
		}
	}
}

func TestProjectAddPattern_RejectsDuplicate(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	pattern := ".cursor/**"
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, pattern); err != nil {
		t.Fatalf("first add: %v", err)
	}

	_, _, err := ps.ProjectAddPattern(context.Background(), repoDir, pattern)
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
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

func TestProjectListPatterns_FromSubdirSeesRootManifest(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	subDir := filepath.Join(repoDir, "sub")
	testhelpers.MakeDir(t, subDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add: %v", err)
	}

	_, local, err := ps.ProjectListPatterns(context.Background(), subDir)
	if err != nil {
		t.Fatalf("ProjectListPatterns: %v", err)
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "agents.md"); err != nil {
		t.Fatalf("add: %v", err)
	}

	removed, isGlobal, err := ps.ProjectUntrackPattern(context.Background(), repoDir, "agents.md")
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
	removed, isGlobal, err := ps.ProjectUntrackPattern(context.Background(), repoDir, ".todo/**")
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
	_, _, err := ps.ProjectUntrackPattern(context.Background(), repoDir, "missing.md")
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
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

func TestProjectPush_FromSubdirStoresRootRelative(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	subDir := filepath.Join(repoDir, "sub")
	testhelpers.MakeDir(t, subDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "notes.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}
	testhelpers.MakeFile(t, filepath.Join(repoDir, "notes.md"), "notes\n")
	testhelpers.MakeFile(t, filepath.Join(subDir, "notes.md"), "sub notes\n")

	result, err := ps.ProjectPush(context.Background(), subDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 2 {
		t.Fatalf("synced = %v, want 2 entries", result.Synced)
	}

	id, err := resolver.ResolveProjectID(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("resolve project id: %v", err)
	}
	for _, rel := range []string{"notes.md", filepath.Join("sub", "notes.md")} {
		storageFile := filepath.Join(svc.RepoPath(), "projects", id, rel)
		if !testhelpers.FileExists(t, storageFile) {
			t.Errorf("expected storage file %s", storageFile)
		}
	}
}

func TestProjectPush_SkipsAlreadySymlinked(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir, false); err != nil {
		t.Fatalf("first push: %v", err)
	}

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
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
	_, err := ps.ProjectPush(context.Background(), repoDir, false)
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "*.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	gitConfig := filepath.Join(repoDir, ".git", "config")
	if _, err := os.Stat(gitConfig); err != nil {
		t.Fatalf("expected .git/config to exist: %v", err)
	}

	liveFile := filepath.Join(repoDir, "README.md")
	testhelpers.MakeFile(t, liveFile, "# hello\n")

	// Force: README.md is committed to the project repo by the fixture.
	result, err := ps.ProjectPush(context.Background(), repoDir, true)
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

func TestProjectPush_SkipsProjectGitTrackedFiles(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "*.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	trackedFile := filepath.Join(repoDir, "README.md") // committed by fixture
	untrackedFile := filepath.Join(repoDir, "notes.md")
	testhelpers.MakeFile(t, untrackedFile, "notes\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 1 || result.Synced[0] != "notes.md" {
		t.Errorf("synced = %v, want [notes.md]", result.Synced)
	}
	if len(result.SkippedTracked) != 1 || result.SkippedTracked[0] != "README.md" {
		t.Errorf("skipped = %v, want [README.md]", result.SkippedTracked)
	}

	// The tracked file must remain a real file, not a symlink.
	info, err := os.Lstat(trackedFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected tracked file to remain a real file")
	}

	// Force manages it anyway.
	forced, err := ps.ProjectPush(context.Background(), repoDir, true)
	if err != nil {
		t.Fatalf("ProjectPush --force: %v", err)
	}
	if len(forced.Synced) != 1 || forced.Synced[0] != "README.md" {
		t.Errorf("forced synced = %v, want [README.md]", forced.Synced)
	}
	storageFile := filepath.Join(svc.RepoPath(), "projects", forced.ProjectID, "README.md")
	testhelpers.AssertSymlink(t, trackedFile, storageFile)
}

func TestProjectPush_PrunesNestedGitRepos(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	nested := filepath.Join(repoDir, "vendor", "lib")
	testhelpers.MakeDir(t, nested)
	testhelpers.InitGitRepo(t, nested)
	nestedFile := filepath.Join(nested, "secret.md")
	testhelpers.MakeFile(t, nestedFile, "nested\n")

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "vendor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 0 {
		t.Errorf("synced = %v, want empty (nested repo pruned)", result.Synced)
	}

	info, err := os.Lstat(nestedFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected nested repo file to remain a real file")
	}
}

func TestProjectPush_NeverStoresLnkMetadata(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "*"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	testhelpers.MakeFile(t, filepath.Join(repoDir, "notes.md"), "notes\n")
	testhelpers.MakeFile(t, filepath.Join(repoDir, "old.md.lnk-backup"), "backup\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if len(result.Synced) != 1 || result.Synced[0] != "notes.md" {
		t.Errorf("synced = %v, want [notes.md]", result.Synced)
	}

	storageDir := filepath.Join(svc.RepoPath(), "projects", result.ProjectID)
	if testhelpers.FileExists(t, filepath.Join(storageDir, ".lnkinclude")) {
		t.Error("expected .lnkinclude to never be stored")
	}
	if testhelpers.FileExists(t, filepath.Join(storageDir, "old.md.lnk-backup")) {
		t.Error("expected .lnk-backup files to never be stored")
	}
	if !testhelpers.FileExists(t, filepath.Join(repoDir, ".lnkinclude")) {
		t.Error("expected live .lnkinclude to remain in place")
	}
}

func TestProjectPush_ReportsMoveFailures(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "blocked.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}
	testhelpers.MakeFile(t, filepath.Join(repoDir, "blocked.md"), "blocked\n")

	// A directory at the storage path makes the move fail.
	id, err := resolver.ResolveProjectID(context.Background(), repoDir)
	if err != nil {
		t.Fatalf("resolve project id: %v", err)
	}
	testhelpers.MakeDir(t, filepath.Join(svc.RepoPath(), "projects", id, "blocked.md"))

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err == nil {
		t.Fatal("expected error when the move fails")
	}
	if !errors.Is(err, lnkerror.ErrSyncFailed) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrSyncFailed)
	}
	if len(result.Synced) != 0 {
		t.Errorf("synced = %v, want empty", result.Synced)
	}

	// The live file must not be lost.
	if !testhelpers.FileExists(t, filepath.Join(repoDir, "blocked.md")) {
		t.Error("expected live file to remain after failed sync")
	}
}

func TestProjectPush_NoOriginUsesLocalID(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "localonly")
	testhelpers.MakeDir(t, repoDir)
	testhelpers.InitGitRepo(t, repoDir) // no origin remote

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, "notes.md"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}
	testhelpers.MakeFile(t, filepath.Join(repoDir, "notes.md"), "notes\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}
	if !strings.HasPrefix(result.ProjectID, "local/") {
		t.Errorf("project id = %q, want local/ prefix", result.ProjectID)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, "notes.md")
	if !testhelpers.FileExists(t, storageFile) {
		t.Errorf("expected storage file %s", storageFile)
	}
}

func TestProjectRestore_RecreatesSymlinks(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectRestore(context.Background(), repoDir, false, false)
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	testhelpers.MakeFile(t, liveFile, "local changes\n")

	info, err := ps.ProjectRestore(context.Background(), repoDir, false, false)
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

func TestProjectRestore_SkipsProjectGitTrackedFiles(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir, false); err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	// Simulate a fresh clone: the live path is a real file tracked upstream.
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	testhelpers.MakeFile(t, liveFile, "upstream content\n")
	if out, err := exec.Command("git", "-C", repoDir, "add", ".cursor/rules.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	info, err := ps.ProjectRestore(context.Background(), repoDir, false, false)
	if err != nil {
		t.Fatalf("ProjectRestore: %v", err)
	}
	if len(info.Restored) != 0 {
		t.Errorf("restored = %v, want empty", info.Restored)
	}
	if len(info.SkippedTracked) != 1 || info.SkippedTracked[0] != ".cursor/rules.md" {
		t.Errorf("skipped = %v, want [.cursor/rules.md]", info.SkippedTracked)
	}
	if len(info.BackedUp) != 0 {
		t.Errorf("backed up = %v, want empty", info.BackedUp)
	}

	liveInfo, err := os.Lstat(liveFile)
	if err != nil {
		t.Fatal(err)
	}
	if liveInfo.Mode()&os.ModeSymlink != 0 {
		t.Error("expected tracked live file to remain a real file")
	}

	// Force replaces it with a symlink, backing the real file up.
	forced, err := ps.ProjectRestore(context.Background(), repoDir, false, true)
	if err != nil {
		t.Fatalf("ProjectRestore --force: %v", err)
	}
	if len(forced.Restored) != 1 || len(forced.BackedUp) != 1 {
		t.Errorf("forced restored = %v, backed up = %v", forced.Restored, forced.BackedUp)
	}
	liveInfo, err = os.Lstat(liveFile)
	if err != nil {
		t.Fatal(err)
	}
	if liveInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink after forced restore")
	}
}

func TestProjectRestore_DryRun(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	initProjectRepo(t, repoDir)

	ps := service.NewProjectService(svc)
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	if _, err := ps.ProjectPush(context.Background(), repoDir, false); err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectRestore(context.Background(), repoDir, true, false)
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
	if _, _, err := ps.ProjectAddPattern(context.Background(), repoDir, ".cursor/**"); err != nil {
		t.Fatalf("add pattern: %v", err)
	}

	liveFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, liveFile, "# rules\n")

	result, err := ps.ProjectPush(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPush: %v", err)
	}

	storageFile := filepath.Join(svc.RepoPath(), "projects", result.ProjectID, ".cursor", "rules.md")
	if err := os.Remove(liveFile); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}

	info, err := ps.ProjectPull(context.Background(), repoDir, false)
	if err != nil {
		t.Fatalf("ProjectPull: %v", err)
	}
	if len(info.Restored) != 1 {
		t.Errorf("restored = %v, want 1 entry", info.Restored)
	}
	testhelpers.AssertSymlink(t, liveFile, storageFile)
}
