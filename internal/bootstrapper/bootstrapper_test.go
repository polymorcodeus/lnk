// internal/bootstrapper/bootstrapper_test.go
package bootstrapper_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polymorcodeus/lnk/internal/bootstrapper"
	"github.com/polymorcodeus/lnk/internal/lnkerror"
)

// --- stubs ---

type fakeGit struct {
	isRepo bool
}

func (f *fakeGit) IsGitRepository() bool { return f.isRepo }

// --- tests ---

func TestRunner_FindScript(t *testing.T) {
	t.Run("returns error when not a git repo", func(t *testing.T) {
		tmp := t.TempDir()
		r := bootstrapper.New(tmp, &fakeGit{isRepo: false})

		_, err := r.FindScript()
		if !errors.Is(err, lnkerror.ErrNotInitialized) {
			t.Fatalf("expected ErrNotInitialized, got %v", err)
		}
	})

	t.Run("finds bootstrap.sh", func(t *testing.T) {
		tmp := t.TempDir()
		// create a fake bootstrap.sh
		script := filepath.Join(tmp, "bootstrap.sh")
		if err := os.WriteFile(script, []byte("#!/bin/bash\necho ok"), 0644); err != nil {
			t.Fatal(err)
		}

		r := bootstrapper.New(tmp, &fakeGit{isRepo: true})
		name, err := r.FindScript()
		if err != nil {
			t.Fatal(err)
		}
		if name != "bootstrap.sh" {
			t.Fatalf("expected bootstrap.sh, got %s", name)
		}
	})
}

func TestRunner_RunScript(t *testing.T) {
	t.Run("executes script successfully", func(t *testing.T) {
		tmp := t.TempDir()
		script := filepath.Join(tmp, "bootstrap.sh")
		os.WriteFile(script, []byte("#!/bin/bash\necho hello"), 0755)

		r := bootstrapper.New(tmp, &fakeGit{isRepo: true})

		var stdout bytes.Buffer
		err := r.RunScript("bootstrap.sh", &stdout, io.Discard, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), "hello") {
			t.Fatalf("expected output to contain hello, got %s", stdout.String())
		}
	})
}
