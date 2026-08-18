package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/polymorcodeus/lnk/internal/lnkerror"
	"github.com/polymorcodeus/lnk/internal/testhelpers"
)

func TestProjectAdd_RefusesHostFile(t *testing.T) {
	svc, home := testhelpers.TestHome(t)

	dotfile := filepath.Join(home, ".vimrc")
	testhelpers.MakeFile(t, dotfile, "# vimrc")

	err := svc.ProjectAdd(context.Background(), []string{dotfile})
	if err == nil {
		t.Fatal("expected error for host file with project add, got nil")
	}
	if !errors.Is(err, lnkerror.ErrOutsideGitRepo) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrOutsideGitRepo)
	}
}

func TestProjectAdd_AcceptsProjectFile(t *testing.T) {
	svc, home := testhelpers.TestHome(t)

	repoDir := filepath.Join(home, "repos", "hermes")
	testhelpers.MakeDir(t, repoDir)
	testhelpers.InitGitRepo(t, repoDir)

	projectFile := filepath.Join(repoDir, ".cursor", "rules.md")
	testhelpers.MakeFile(t, projectFile, "# rules")

	err := svc.ProjectAdd(context.Background(), []string{projectFile})
	if err == nil {
		t.Fatal("expected error for unimplemented project scope, got nil")
	}
	if !errors.Is(err, lnkerror.ErrProjectScopeNotImplemented) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrProjectScopeNotImplemented)
	}
}

func TestProjectAdd_EmptyPaths(t *testing.T) {
	svc, _ := testhelpers.TestHome(t)

	err := svc.ProjectAdd(context.Background(), []string{})
	if err == nil {
		t.Fatal("expected error for empty paths, got nil")
	}
	if !errors.Is(err, lnkerror.ErrNoPaths) {
		t.Errorf("error = %v, want %v", err, lnkerror.ErrNoPaths)
	}
}
