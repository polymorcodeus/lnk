// internal/tracker/tracker_test.go
package tracker_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/polymorcodeus/lnk/internal/tracker"
)

func TestTracker_LnkFileName(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		format tracker.RepoFormat
		want   string
		err    bool
	}{
		{"common v2", "common", tracker.FormatV2, ".lnk.common", false},
		{"common v1", "common", tracker.FormatV1, ".lnk", false},
		{"common unknown", "common", tracker.FormatUnknown, "", true},
		{"host v2", "myhost", tracker.FormatV2, ".lnk.myhost", false},
		{"host v1", "myhost", tracker.FormatV1, ".lnk.myhost", false},
		{"host unknown", "myhost", tracker.FormatUnknown, ".lnk.myhost", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tracker.New("repo", tt.host, tt.format)
			got, err := tr.LnkFileName()
			if tt.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LnkFileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTracker_HostStoragePath(t *testing.T) {
	tmp := t.TempDir()

	tests := []struct {
		name   string
		host   string
		format tracker.RepoFormat
		want   string
		err    bool
	}{
		{"common v2", "common", tracker.FormatV2, filepath.Join(tmp, "common.lnk"), false},
		{"common v1", "common", tracker.FormatV1, tmp, false},
		{"common unknown", "common", tracker.FormatUnknown, "", true},
		{"host v2", "myhost", tracker.FormatV2, filepath.Join(tmp, "myhost.lnk"), false},
		{"host v1", "myhost", tracker.FormatV1, filepath.Join(tmp, "myhost.lnk"), false},
		{"host unknown", "myhost", tracker.FormatUnknown, filepath.Join(tmp, "myhost.lnk"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tracker.New(tmp, tt.host, tt.format)
			got, err := tr.HostStoragePath()
			if tt.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("HostStoragePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTracker_GetManagedItems(t *testing.T) {
	tmp := t.TempDir()
	tr := tracker.New(tmp, "common", tracker.FormatV2)

	t.Run("file does not exist", func(t *testing.T) {
		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty, got %v", items)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		lnk := filepath.Join(tmp, ".lnk.common")
		os.WriteFile(lnk, []byte(""), 0644)

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty, got %v", items)
		}
	})

	t.Run("parses lines and trims whitespace", func(t *testing.T) {
		lnk := filepath.Join(tmp, ".lnk.common")
		content := "foo\n\n  bar  \nbaz\n"
		os.WriteFile(lnk, []byte(content), 0644)

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"foo", "bar", "baz"} // file order, not sorted
		if !slices.Equal(items, want) {
			t.Errorf("got %v, want %v", items, want)
		}
	})
}

func TestTracker_AddManagedItem(t *testing.T) {
	tmp := t.TempDir()
	tr := tracker.New(tmp, "common", tracker.FormatV2)

	t.Run("adds first item", func(t *testing.T) {
		if err := tr.AddManagedItem("foo"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(items, []string{"foo"}) {
			t.Errorf("got %v", items)
		}
	})

	t.Run("adds second item and sorts", func(t *testing.T) {
		if err := tr.AddManagedItem("baz"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"baz", "foo"}
		if !slices.Equal(items, want) {
			t.Errorf("got %v, want %v", items, want)
		}
	})

	t.Run("duplicate is no-op", func(t *testing.T) {
		if err := tr.AddManagedItem("foo"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"baz", "foo"}
		if !slices.Equal(items, want) {
			t.Errorf("got %v, want %v", items, want)
		}
	})
}

func TestTracker_RemoveManagedItem(t *testing.T) {
	tmp := t.TempDir()
	tr := tracker.New(tmp, "common", tracker.FormatV2)

	// Seed tracker
	if err := tr.WriteManagedItems([]string{"alpha", "beta", "gamma"}); err != nil {
		t.Fatal(err)
	}

	t.Run("removes existing item", func(t *testing.T) {
		if err := tr.RemoveManagedItem("beta"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"alpha", "gamma"}
		if !slices.Equal(items, want) {
			t.Errorf("got %v, want %v", items, want)
		}
	})

	t.Run("nonexistent item is no-op", func(t *testing.T) {
		if err := tr.RemoveManagedItem("delta"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"alpha", "gamma"}
		if !slices.Equal(items, want) {
			t.Errorf("got %v, want %v", items, want)
		}
	})

	t.Run("removes last item", func(t *testing.T) {
		if err := tr.RemoveManagedItem("alpha"); err != nil {
			t.Fatal(err)
		}
		if err := tr.RemoveManagedItem("gamma"); err != nil {
			t.Fatal(err)
		}

		items, err := tr.GetManagedItems()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty, got %v", items)
		}
		// Verify file is empty (0 bytes)
		lnk := filepath.Join(tmp, ".lnk.common")
		info, err := os.Stat(lnk)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != 0 {
			t.Errorf("expected empty file, got %d bytes", info.Size())
		}
	})
}

func TestTracker_WriteManagedItems(t *testing.T) {
	tmp := t.TempDir()
	tr := tracker.New(tmp, "common", tracker.FormatV2)

	t.Run("writes items with trailing newline", func(t *testing.T) {
		if err := tr.WriteManagedItems([]string{"foo", "bar"}); err != nil {
			t.Fatal(err)
		}

		lnk := filepath.Join(tmp, ".lnk.common")
		content, err := os.ReadFile(lnk)
		if err != nil {
			t.Fatal(err)
		}
		want := "foo\nbar\n" // input order, not sorted
		if string(content) != want {
			t.Errorf("content = %q, want %q", string(content), want)
		}
	})

	t.Run("empty slice writes empty file", func(t *testing.T) {
		if err := tr.WriteManagedItems([]string{}); err != nil {
			t.Fatal(err)
		}

		lnk := filepath.Join(tmp, ".lnk.common")
		content, err := os.ReadFile(lnk)
		if err != nil {
			t.Fatal(err)
		}
		if len(content) != 0 {
			t.Errorf("expected empty file, got %q", string(content))
		}
	})
}
