// internal/filemanager/filemanager_test.go
package filemanager_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/polymorcodeus/lnk/internal/filemanager"
)

// ---------- fakes ----------

type fakeFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	dir     bool
}

func (f *fakeFileInfo) Name() string       { return f.name }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f *fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f *fakeFileInfo) IsDir() bool        { return f.dir }
func (f *fakeFileInfo) Sys() interface{}   { return nil }

type moveCall struct{ src, dst string }

type fakeFileSystem struct {
	validateFileInfoFunc func(path string) (os.FileInfo, error)
	moveFunc             func(src, dst string, info os.FileInfo) error
	moveCalls            []moveCall
	createSymlinkFunc    func(target, link string) error
	validateSymlinkFunc  func(absPath, repoPath string) error
}

func (f *fakeFileSystem) ValidateFileInfoForAdd(path string) (os.FileInfo, error) {
	if f.validateFileInfoFunc != nil {
		return f.validateFileInfoFunc(path)
	}
	return nil, errors.New("not implemented")
}

func (f *fakeFileSystem) Move(src, dst string, info os.FileInfo) error {
	f.moveCalls = append(f.moveCalls, moveCall{src, dst})
	if f.moveFunc != nil {
		return f.moveFunc(src, dst, info)
	}
	return nil
}

func (f *fakeFileSystem) CreateSymlink(target, link string) error {
	if f.createSymlinkFunc != nil {
		return f.createSymlinkFunc(target, link)
	}
	return nil
}

func (f *fakeFileSystem) ValidateSymlinkForRemove(absPath, repoPath string) error {
	if f.validateSymlinkFunc != nil {
		return f.validateSymlinkFunc(absPath, repoPath)
	}
	return nil
}

type fakeTracker struct {
	lnkFileNameFunc       func() (string, error)
	hostStoragePathFunc   func() (string, error)
	addManagedItemFunc    func(path string) error
	removeManagedItemFunc func(path string) error
	getManagedItemsFunc   func() ([]string, error)
}

func (f *fakeTracker) LnkFileName() (string, error) {
	if f.lnkFileNameFunc != nil {
		return f.lnkFileNameFunc()
	}
	return "", errors.New("not implemented")
}

func (f *fakeTracker) HostStoragePath() (string, error) {
	if f.hostStoragePathFunc != nil {
		return f.hostStoragePathFunc()
	}
	return "", errors.New("not implemented")
}

func (f *fakeTracker) AddManagedItem(path string) error {
	if f.addManagedItemFunc != nil {
		return f.addManagedItemFunc(path)
	}
	return nil
}

func (f *fakeTracker) RemoveManagedItem(path string) error {
	if f.removeManagedItemFunc != nil {
		return f.removeManagedItemFunc(path)
	}
	return nil
}

func (f *fakeTracker) GetManagedItems() ([]string, error) {
	if f.getManagedItemsFunc != nil {
		return f.getManagedItemsFunc()
	}
	return nil, errors.New("not implemented")
}

// ---------- tests ----------

func TestManager_AddMultiple(t *testing.T) {
	t.Run("empty paths returns empty result", func(t *testing.T) {
		fm := filemanager.New("repo", "host", &fakeFileSystem{}, &fakeTracker{})
		result, err := fm.AddMultiple(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.StagePaths) != 0 {
			t.Errorf("expected empty StagePaths, got %v", result.StagePaths)
		}
		if len(result.Rollback) != 0 {
			t.Errorf("expected empty Rollback, got %d", len(result.Rollback))
		}
	})

	t.Run("validation failure returns error", func(t *testing.T) {
		fs := &fakeFileSystem{
			validateFileInfoFunc: func(path string) (os.FileInfo, error) {
				return nil, errors.New("not a regular file")
			},
		}
		fm := filemanager.New("repo", "host", fs, &fakeTracker{})
		_, err := fm.AddMultiple([]filemanager.FileToTrack{
			{AbsPath: "/foo", RelativePath: "foo"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("success returns stage paths and rollback", func(t *testing.T) {
		tmp := t.TempDir()
		storage := filepath.Join(tmp, "storage")

		fs := &fakeFileSystem{
			validateFileInfoFunc: func(path string) (os.FileInfo, error) {
				return &fakeFileInfo{name: filepath.Base(path), mode: 0644}, nil
			},
			moveFunc:          func(src, dst string, info os.FileInfo) error { return nil },
			createSymlinkFunc: func(target, link string) error { return nil },
		}

		trk := &fakeTracker{
			lnkFileNameFunc: func() (string, error) {
				return filepath.Join("repo", ".lnk"), nil
			},
			hostStoragePathFunc: func() (string, error) { return storage, nil },
			addManagedItemFunc:  func(path string) error { return nil },
		}

		fm := filemanager.New("repo", "host", fs, trk)
		result, err := fm.AddMultiple([]filemanager.FileToTrack{
			{AbsPath: "/foo", RelativePath: "foo"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wantPaths := []string{filepath.Join(storage, "foo"), filepath.Join("repo", ".lnk")}
		if len(result.StagePaths) != len(wantPaths) {
			t.Fatalf("expected %d stage paths, got %d: %v", len(wantPaths), len(result.StagePaths), result.StagePaths)
		}
		if result.StagePaths[0] != wantPaths[0] {
			t.Errorf("stage path[0] = %q, want %q", result.StagePaths[0], wantPaths[0])
		}
		if result.StagePaths[1] != wantPaths[1] {
			t.Errorf("stage path[1] = %q, want %q", result.StagePaths[1], wantPaths[1])
		}
		if len(result.Rollback) != 1 {
			t.Errorf("expected 1 rollback action, got %d", len(result.Rollback))
		}
	})

	t.Run("rollback on move failure", func(t *testing.T) {
		tmp := t.TempDir()
		storage := filepath.Join(tmp, "storage")

		fs := &fakeFileSystem{
			validateFileInfoFunc: func(path string) (os.FileInfo, error) {
				return &fakeFileInfo{name: filepath.Base(path), mode: 0644}, nil
			},
			moveFunc: func(src, dst string, info os.FileInfo) error {
				if strings.Contains(dst, "file2") {
					return errors.New("disk full")
				}
				return nil
			},
			createSymlinkFunc: func(target, link string) error { return nil },
		}

		trk := &fakeTracker{
			lnkFileNameFunc:     func() (string, error) { return ".lnk", nil },
			hostStoragePathFunc: func() (string, error) { return storage, nil },
			addManagedItemFunc:  func(path string) error { return nil },
		}

		fm := filemanager.New("repo", "host", fs, trk)
		_, err := fm.AddMultiple([]filemanager.FileToTrack{
			{AbsPath: "/file1", RelativePath: "file1"},
			{AbsPath: "/file2", RelativePath: "file2"},
		})
		if err == nil {
			t.Fatal("expected error")
		}

		// 1. move file1 forward
		// 2. move file2 forward (fails)
		// 3. rollback file1 (move back)
		if len(fs.moveCalls) != 3 {
			t.Fatalf("expected 3 move calls, got %d: %v", len(fs.moveCalls), fs.moveCalls)
		}
		if fs.moveCalls[2].src != filepath.Join(storage, "file1") {
			t.Errorf("rollback src = %q, want %q", fs.moveCalls[2].src, filepath.Join(storage, "file1"))
		}
		if fs.moveCalls[2].dst != "/file1" {
			t.Errorf("rollback dst = %q, want %q", fs.moveCalls[2].dst, "/file1")
		}
	})

	t.Run("rollback on symlink failure", func(t *testing.T) {
		tmp := t.TempDir()
		storage := filepath.Join(tmp, "storage")

		fs := &fakeFileSystem{
			validateFileInfoFunc: func(path string) (os.FileInfo, error) {
				return &fakeFileInfo{name: filepath.Base(path), mode: 0644}, nil
			},
			moveFunc:          func(src, dst string, info os.FileInfo) error { return nil },
			createSymlinkFunc: func(target, link string) error { return errors.New("permission denied") },
		}

		trk := &fakeTracker{
			lnkFileNameFunc:     func() (string, error) { return ".lnk", nil },
			hostStoragePathFunc: func() (string, error) { return storage, nil },
			addManagedItemFunc:  func(path string) error { return nil },
		}

		fm := filemanager.New("repo", "host", fs, trk)
		_, err := fm.AddMultiple([]filemanager.FileToTrack{
			{AbsPath: "/foo", RelativePath: "foo"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Errorf("unexpected error: %v", err)
		}
		// forward move + rollback move
		if len(fs.moveCalls) != 2 {
			t.Errorf("expected 2 move calls, got %d", len(fs.moveCalls))
		}
	})

	t.Run("rollback on tracker failure", func(t *testing.T) {
		tmp := t.TempDir()
		storage := filepath.Join(tmp, "storage")

		fs := &fakeFileSystem{
			validateFileInfoFunc: func(path string) (os.FileInfo, error) {
				return &fakeFileInfo{name: filepath.Base(path), mode: 0644}, nil
			},
			moveFunc:          func(src, dst string, info os.FileInfo) error { return nil },
			createSymlinkFunc: func(target, link string) error { return nil },
		}

		trk := &fakeTracker{
			lnkFileNameFunc:     func() (string, error) { return ".lnk", nil },
			hostStoragePathFunc: func() (string, error) { return storage, nil },
			addManagedItemFunc: func(path string) error {
				return errors.New("tracker locked")
			},
		}

		fm := filemanager.New("repo", "host", fs, trk)
		_, err := fm.AddMultiple([]filemanager.FileToTrack{
			{AbsPath: "/foo", RelativePath: "foo"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "tracking file") {
			t.Errorf("unexpected error: %v", err)
		}
		// forward move + rollback move
		if len(fs.moveCalls) != 2 {
			t.Errorf("expected 2 move calls, got %d", len(fs.moveCalls))
		}
	})
}

func TestManager_RollbackAll(t *testing.T) {
	callOrder := []int{}
	actions := []func() error{
		func() error { callOrder = append(callOrder, 1); return nil },
		func() error { callOrder = append(callOrder, 2); return nil },
		func() error { callOrder = append(callOrder, 3); return nil },
	}

	fm := filemanager.New("repo", "host", &fakeFileSystem{}, &fakeTracker{})
	fm.RollbackAll(actions)

	want := []int{3, 2, 1}
	if len(callOrder) != len(want) {
		t.Fatalf("expected %d calls, got %d", len(want), len(callOrder))
	}
	for i := range want {
		if callOrder[i] != want[i] {
			t.Errorf("callOrder[%d] = %d, want %d", i, callOrder[i], want[i])
		}
	}
}

func TestManager_Remove(t *testing.T) {
	// ... other subtests stay the same ...

	t.Run("success removes symlink and returns restore function", func(t *testing.T) {
		tmp := t.TempDir()
		repoPath := filepath.Join(tmp, "repo")
		os.MkdirAll(repoPath, 0755)

		target := filepath.Join(tmp, "storage", "foo")
		os.MkdirAll(filepath.Dir(target), 0755)
		os.WriteFile(target, []byte("hello"), 0644)
		link := filepath.Join(tmp, "link")
		os.Symlink(target, link)

		fs := &fakeFileSystem{
			validateSymlinkFunc: func(absPath, repoPath string) error { return nil },
			// Make the fake actually move files so we can verify end-to-end state
			moveFunc: func(src, dst string, info os.FileInfo) error {
				return os.Rename(src, dst)
			},
		}
		trk := &fakeTracker{
			lnkFileNameFunc:       func() (string, error) { return ".lnk", nil },
			getManagedItemsFunc:   func() ([]string, error) { return []string{"link"}, nil },
			removeManagedItemFunc: func(path string) error { return nil },
		}

		fm := filemanager.New(repoPath, "host", fs, trk)
		result, err := fm.Remove(filemanager.FileToTrack{AbsPath: link, RelativePath: "link"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.StagePaths) != 1 || result.StagePaths[0] != ".lnk" {
			t.Errorf("unexpected stage paths: %v", result.StagePaths)
		}
		if len(result.RemovePaths) != 1 || result.RemovePaths[0] != target {
			t.Errorf("unexpected remove paths: %v", result.RemovePaths)
		}

		// Symlink should be gone
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Error("expected symlink to be removed")
		}

		// Restore should move file back
		if result.RestoreFn == nil {
			t.Fatal("expected RestoreFn")
		}
		if err := result.RestoreFn(); err != nil {
			t.Fatalf("restore failed: %v", err)
		}
		if _, err := os.Stat(link); err != nil {
			t.Errorf("expected file to be restored at %s: %v", link, err)
		}
	})
}
