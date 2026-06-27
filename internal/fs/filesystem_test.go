// internal/fs/filesystem_test.go
package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/polymorcodeus/lnk/internal/fs"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

func TestFileSystem_ValidateFileInfoForAdd(t *testing.T) {
	fsys := fs.New()

	t.Run("file not found", func(t *testing.T) {
		_, err := fsys.ValidateFileInfoForAdd("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, fs.ErrFileNotExists) {
			t.Errorf("expected ErrFileNotExists, got %v", err)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "file.txt")
		os.WriteFile(path, []byte("hello"), 0644)

		info, err := fsys.ValidateFileInfoForAdd(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Name() != "file.txt" {
			t.Errorf("unexpected name: %s", info.Name())
		}
	})

	t.Run("directory", func(t *testing.T) {
		tmp := t.TempDir()

		info, err := fsys.ValidateFileInfoForAdd(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
	})

	t.Run("symlink rejected", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "target")
		link := filepath.Join(tmp, "link")
		os.WriteFile(target, []byte("x"), 0644)
		os.Symlink(target, link)

		_, err := fsys.ValidateFileInfoForAdd(link)
		if err == nil {
			t.Fatal("expected error for symlink")
		}
		if !errors.Is(err, fs.ErrUnsupportedType) {
			t.Errorf("expected ErrUnsupportedType, got %v", err)
		}
	})
}

func TestFileSystem_ValidateSymlinkForRemove(t *testing.T) {
	fsys := fs.New()

	t.Run("not a symlink", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "file.txt")
		os.WriteFile(path, []byte("x"), 0644)

		err := fsys.ValidateSymlinkForRemove(path, tmp)
		if !errors.Is(err, lnkerror.ErrNotManaged) {
			t.Fatalf("expected ErrNotManaged, got %v", err)
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		err := fsys.ValidateSymlinkForRemove("/nonexistent", "/repo")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, fs.ErrFileNotExists) {
			t.Errorf("expected ErrFileNotExists, got %v", err)
		}
	})

	t.Run("symlink outside repo", func(t *testing.T) {
		tmp := t.TempDir()
		repo := filepath.Join(tmp, "repo")
		os.MkdirAll(repo, 0755)

		outside := filepath.Join(tmp, "outside")
		link := filepath.Join(tmp, "link")
		os.WriteFile(outside, []byte("x"), 0644)
		os.Symlink(outside, link)

		err := fsys.ValidateSymlinkForRemove(link, repo)
		if !errors.Is(err, lnkerror.ErrNotManaged) {
			t.Fatalf("expected ErrNotManaged, got %v", err)
		}
	})

	t.Run("symlink inside repo", func(t *testing.T) {
		tmp := t.TempDir()
		repo := filepath.Join(tmp, "repo")
		storage := filepath.Join(repo, "storage")
		os.MkdirAll(storage, 0755)

		target := filepath.Join(storage, "file.txt")
		link := filepath.Join(tmp, "link")
		os.WriteFile(target, []byte("x"), 0644)
		os.Symlink(target, link)

		err := fsys.ValidateSymlinkForRemove(link, repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("relative symlink inside repo", func(t *testing.T) {
		tmp := t.TempDir()
		repo := filepath.Join(tmp, "repo")
		storage := filepath.Join(repo, "storage")
		os.MkdirAll(storage, 0755)

		target := filepath.Join(storage, "file.txt")
		link := filepath.Join(repo, "link.txt")
		os.WriteFile(target, []byte("x"), 0644)
		// relative symlink: link.txt -> storage/file.txt
		rel, _ := filepath.Rel(repo, target)
		os.Symlink(rel, link)

		err := fsys.ValidateSymlinkForRemove(link, repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFileSystem_MoveFile(t *testing.T) {
	fsys := fs.New()

	t.Run("moves file and creates parent dirs", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src.txt")
		dst := filepath.Join(tmp, "nested", "dst.txt")
		os.WriteFile(src, []byte("hello"), 0644)

		err := fsys.MoveFile(src, dst)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("expected src to not exist")
		}
		content, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "hello" {
			t.Errorf("unexpected content: %s", content)
		}
	})
}

func TestFileSystem_MoveDirectory(t *testing.T) {
	fsys := fs.New()

	t.Run("moves directory with contents", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "moved", "dst")
		os.MkdirAll(filepath.Join(src, "subdir"), 0755)
		os.WriteFile(filepath.Join(src, "file.txt"), []byte("x"), 0644)

		err := fsys.MoveDirectory(src, dst)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("expected src to not exist")
		}
		if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
			t.Errorf("expected file in dst: %v", err)
		}
	})
}

func TestFileSystem_Move(t *testing.T) {
	fsys := fs.New()

	t.Run("delegates to MoveFile for files", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "file.txt")
		dst := filepath.Join(tmp, "moved.txt")
		os.WriteFile(src, []byte("x"), 0644)

		info, _ := os.Stat(src)
		err := fsys.Move(src, dst, info)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("delegates to MoveDirectory for dirs", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "dir")
		dst := filepath.Join(tmp, "moved", "dir")
		os.MkdirAll(src, 0755)

		info, _ := os.Stat(src)
		err := fsys.Move(src, dst, info)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFileSystem_CreateSymlink(t *testing.T) {
	fsys := fs.New()

	t.Run("creates relative symlink", func(t *testing.T) {
		tmp := t.TempDir()
		target := filepath.Join(tmp, "storage", "file.txt")
		link := filepath.Join(tmp, "link.txt")
		os.MkdirAll(filepath.Dir(target), 0755)
		os.WriteFile(target, []byte("x"), 0644)

		err := fsys.CreateSymlink(target, link)
		if err != nil {
			t.Fatal(err)
		}

		got, err := os.Readlink(link)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.IsAbs(got) {
			t.Errorf("expected relative symlink, got absolute: %s", got)
		}
		// filepath.Rel behavior varies by platform
		want := filepath.Join("storage", "file.txt")
		if got != want {
			t.Errorf("symlink target = %q, want %q", got, want)
		}
	})
}

func TestRemoveEmptyDirs(t *testing.T) {
	t.Run("removes nested empty directories", func(t *testing.T) {
		tmp := t.TempDir()
		empty1 := filepath.Join(tmp, "a", "b", "c")
		empty2 := filepath.Join(tmp, "a", "b")
		keep := filepath.Join(tmp, "a", "keep")
		os.MkdirAll(empty1, 0755)
		os.MkdirAll(keep, 0755)
		os.WriteFile(filepath.Join(keep, "file.txt"), []byte("x"), 0644)

		err := fs.RemoveEmptyDirs(tmp)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(empty1); !os.IsNotExist(err) {
			t.Error("expected empty1 to be removed")
		}
		if _, err := os.Stat(empty2); !os.IsNotExist(err) {
			t.Error("expected empty2 to be removed")
		}
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("expected keep to exist: %v", err)
		}
	})

	t.Run("keeps non-empty root", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("x"), 0644)

		err := fs.RemoveEmptyDirs(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(tmp); err != nil {
			t.Errorf("expected root to exist: %v", err)
		}
	})

	t.Run("removes sibling empty dirs", func(t *testing.T) {
		tmp := t.TempDir()
		emptyA := filepath.Join(tmp, "emptyA")
		emptyB := filepath.Join(tmp, "emptyB")
		full := filepath.Join(tmp, "full")
		os.MkdirAll(emptyA, 0755)
		os.MkdirAll(emptyB, 0755)
		os.MkdirAll(full, 0755)
		os.WriteFile(filepath.Join(full, "f.txt"), []byte("x"), 0644)

		err := fs.RemoveEmptyDirs(tmp)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(emptyA); !os.IsNotExist(err) {
			t.Error("expected emptyA to be removed")
		}
		if _, err := os.Stat(emptyB); !os.IsNotExist(err) {
			t.Error("expected emptyB to be removed")
		}
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected full to exist: %v", err)
		}
	})
}
