package service_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polymorcodeus/lnk/internal/testhelpers"
)

// setupTrackedFile builds the on-disk state for a tracked file without going
// through Add. It writes the file to repo storage, creates the symlink at the
// live path, writes the relative path into the tracker file, and commits
// everything to git. Returns the storage path and live path.
func setupTrackedFile(t *testing.T, repoPath, home, scope, relativePath, content string) (storagePath, livePath string) {
	t.Helper()

	// Determine storage root and tracker name based on scope and format.
	// For v2 common, common.lnk may not exist yet (it's created on first Add),
	// so we detect format from the marker file content rather than directory presence.
	var storageRoot string
	if scope == "" || scope == "common" {
		// Detect v1 vs v2 from the marker file.
		marker, _ := os.ReadFile(filepath.Join(repoPath, ".lnkrepo"))
		if strings.Contains(string(marker), "version=1") {
			storageRoot = repoPath
		} else if len(marker) == 0 {
			// No marker — legacy v1 detected by presence of .lnk tracker.
			if _, err := os.Stat(filepath.Join(repoPath, ".lnk")); err == nil {
				storageRoot = repoPath
			} else {
				storageRoot = filepath.Join(repoPath, "common.lnk")
			}
		} else {
			storageRoot = filepath.Join(repoPath, "common.lnk")
		}
	} else {
		storageRoot = filepath.Join(repoPath, scope+".lnk")
	}

	storagePath = filepath.Join(storageRoot, relativePath)
	livePath = filepath.Join(home, relativePath)

	// Write file to repo storage.
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storagePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink at live path pointing to storage.
	if err := os.MkdirAll(filepath.Dir(livePath), 0755); err != nil {
		t.Fatal(err)
	}
	relTarget, err := filepath.Rel(filepath.Dir(livePath), storagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(relTarget, livePath); err != nil {
		t.Fatal(err)
	}

	// Determine tracker file name from storage root.
	var trackerName string
	if scope == "" || scope == "common" {
		if storageRoot == repoPath {
			trackerName = ".lnk" // v1 or v1 legacy
		} else {
			trackerName = ".lnk.common" // v2
		}
	} else {
		trackerName = ".lnk." + scope
	}

	// Append relative path to tracker file.
	trackerPath := filepath.Join(repoPath, trackerName)
	existing, _ := os.ReadFile(trackerPath)
	entries := strings.TrimSpace(string(existing))
	if entries != "" {
		entries += "\n"
	}
	entries += relativePath + "\n"
	if err := os.WriteFile(trackerPath, []byte(entries), 0644); err != nil {
		t.Fatal(err)
	}

	// Commit state to git.
	commitCmds := [][]string{
		{"git", "-C", repoPath, "add", "."},
		{"git", "-C", repoPath, "commit", "-m", "lnk: added " + relativePath},
	}
	for _, c := range commitCmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", c, err, out)
		}
	}

	return storagePath, livePath
}

// ---------- Remove tests ----------

func TestRemove_CommonScope_EmptyHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Remove(context.Background(), "", livePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Live path should be a regular file again, not a symlink.
	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat live path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected live path to be a regular file after remove, not a symlink")
	}

	// Storage path should no longer exist.
	if testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage path to be removed from repo")
	}

	// Tracker should no longer contain the entry.
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")

	// Should have a new commit.
	commits := testhelpers.GitLog(t, repoPath)
	if len(commits) < 3 {
		t.Errorf("expected at least 3 commits (init + add + remove), got %d", len(commits))
	}
}

func TestRemove_CommonScope_ExplicitCommonHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Remove(context.Background(), "common", livePath); err != nil {
		t.Fatalf("Remove with explicit common: %v", err)
	}

	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")

	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat live path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected live path to be a regular file after remove")
	}
}

func TestRemove_HostScope(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	if err := svc.Remove(context.Background(), "testhost", livePath); err != nil {
		t.Fatalf("Remove host scope: %v", err)
	}

	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat live path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected live path to be a regular file after remove")
	}

	if testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage path to be removed from repo")
	}

	testhelpers.AssertNotTrackedInScope(t, repoPath, "testhost", ".bashrc")
}

func TestRemove_NestedFile(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "common", ".config/git/config", "[user]")

	if err := svc.Remove(context.Background(), "", livePath); err != nil {
		t.Fatalf("Remove nested: %v", err)
	}

	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat live path: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected live path to be a regular file after remove")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".config/git/config")
}

func TestRemove_FileRestoredAfterCommit(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc content")

	if err := svc.Remove(context.Background(), "", livePath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify the restored file has the original content.
	content, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("ReadFile restored: %v", err)
	}
	if string(content) != "# bashrc content" {
		t.Errorf("restored content = %q, want %q", string(content), "# bashrc content")
	}

	// Verify the restore happened after the git commit.
	commits := testhelpers.GitLog(t, repoPath)
	if len(commits) < 3 {
		t.Errorf("expected at least 3 commits, got %d", len(commits))
	}
}

func TestRemove_NotManaged(t *testing.T) {
	svc, home := testhelpers.TestHome(t)

	filePath := filepath.Join(home, ".bashrc")
	testhelpers.MakeFile(t, filePath, "# bashrc")

	err := svc.Remove(context.Background(), "", filePath)
	if err == nil {
		t.Fatal("expected error removing unmanaged file, got nil")
	}
}

func TestRemove_WrongScope(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	// File is in common but we specify a host scope.
	err := svc.Remove(context.Background(), "testhost", livePath)
	if err == nil {
		t.Fatal("expected error removing file from wrong scope, got nil")
	}
}

func TestRemove_V1_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Remove(context.Background(), "", livePath); err != nil {
		t.Fatalf("Remove v1: %v", err)
	}

	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected regular file after remove")
	}
	if testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage path removed")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

func TestRemove_V1Legacy_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1Legacy(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Remove(context.Background(), "", livePath); err != nil {
		t.Fatalf("Remove v1 legacy: %v", err)
	}

	info, err := os.Lstat(livePath)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected regular file after remove")
	}
	if testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage path removed")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

// ---------- Forget tests ----------

func TestForget_CommonScope_EmptyHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Forget(context.Background(), "", livePath); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// Live symlink should be gone.
	if testhelpers.FileExists(t, livePath) {
		t.Error("expected live symlink to be removed after forget")
	}

	// Storage file should still exist.
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain after forget")
	}

	// Tracker should no longer contain the entry.
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")

	commits := testhelpers.GitLog(t, repoPath)
	if len(commits) < 3 {
		t.Errorf("expected at least 3 commits (init + add + forget), got %d", len(commits))
	}
}

func TestForget_CommonScope_ExplicitCommonHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Forget(context.Background(), "common", livePath); err != nil {
		t.Fatalf("Forget with explicit common: %v", err)
	}

	if testhelpers.FileExists(t, livePath) {
		t.Error("expected live symlink to be removed")
	}
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

func TestForget_HostScope(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	if err := svc.Forget(context.Background(), "testhost", livePath); err != nil {
		t.Fatalf("Forget host scope: %v", err)
	}

	if testhelpers.FileExists(t, livePath) {
		t.Error("expected live symlink removed")
	}
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain")
	}
	testhelpers.AssertNotTrackedInScope(t, repoPath, "testhost", ".bashrc")
}

func TestForget_SymlinkAlreadyGone(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	// Remove the symlink manually before calling Forget.
	if err := os.Remove(livePath); err != nil {
		t.Fatal(err)
	}

	// Forget should still succeed — symlink being gone is not an error.
	if err := svc.Forget(context.Background(), "", livePath); err != nil {
		t.Fatalf("Forget with missing symlink: %v", err)
	}

	// Storage should still exist.
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

func TestForget_NotManaged(t *testing.T) {
	svc, home := testhelpers.TestHome(t)

	filePath := filepath.Join(home, ".bashrc")
	testhelpers.MakeFile(t, filePath, "# bashrc")

	err := svc.Forget(context.Background(), "", filePath)
	if err == nil {
		t.Fatal("expected error forgetting unmanaged file, got nil")
	}
}

func TestForget_WrongScope(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	err := svc.Forget(context.Background(), "testhost", livePath)
	if err == nil {
		t.Fatal("expected error forgetting file from wrong scope, got nil")
	}
}

func TestForget_V1_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Forget(context.Background(), "", livePath); err != nil {
		t.Fatalf("Forget v1: %v", err)
	}

	if testhelpers.FileExists(t, livePath) {
		t.Error("expected live symlink removed")
	}
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

func TestForget_V1Legacy_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1Legacy(t)
	repoPath := svc.RepoPath()

	storagePath, livePath := setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	if err := svc.Forget(context.Background(), "", livePath); err != nil {
		t.Fatalf("Forget v1 legacy: %v", err)
	}

	if testhelpers.FileExists(t, livePath) {
		t.Error("expected live symlink removed")
	}
	if !testhelpers.FileExists(t, storagePath) {
		t.Error("expected storage file to remain")
	}
	testhelpers.AssertNotTracked(t, repoPath, ".bashrc")
}

func TestRemove_HostScopedFile_WithEmptyHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	// Passing "" should fail and tell the user to specify --host.
	err := svc.Remove(context.Background(), "", livePath)
	if err == nil {
		t.Fatal("expected error removing host-scoped file without --host, got nil")
	}
	if !strings.Contains(err.Error(), "--host") {
		t.Errorf("error = %q, want mention of --host", err.Error())
	}
}

func TestRemove_HostScopedFile_WithCommonHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	// Passing "common" explicitly should also fail — file is not in common scope.
	err := svc.Remove(context.Background(), "common", livePath)
	if err == nil {
		t.Fatal("expected error removing host-scoped file with common host, got nil")
	}
}

func TestForget_HostScopedFile_WithEmptyHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	err := svc.Forget(context.Background(), "", livePath)
	if err == nil {
		t.Fatal("expected error forgetting host-scoped file without --host, got nil")
	}
	if !strings.Contains(err.Error(), "--host") {
		t.Errorf("error = %q, want mention of --host", err.Error())
	}
}

func TestForget_HostScopedFile_WithCommonHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	_, livePath := setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	err := svc.Forget(context.Background(), "common", livePath)
	if err == nil {
		t.Fatal("expected error forgetting host-scoped file with common host, got nil")
	}
}
