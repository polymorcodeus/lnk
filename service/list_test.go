package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/polymorcodeus/lnk/internal/testhelpers"
	"github.com/polymorcodeus/lnk/service"
)

// ---------- List tests ----------

func TestList_CommonScope_EmptyHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")
	setupTrackedFile(t, repoPath, home, "common", ".vimrc", "\" vimrc")

	result, err := svc.List(context.Background(), "", false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if result.Scopes[0].Name != "common" {
		t.Errorf("scope name = %q, want %q", result.Scopes[0].Name, "common")
	}
	if len(result.Scopes[0].Items) != 2 {
		t.Fatalf("expected 2 items, got %d: %v", len(result.Scopes[0].Items), result.Scopes[0].Items)
	}
	// Tracker writes items in sorted order.
	if result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("items[0] = %q, want .bashrc", result.Scopes[0].Items[0])
	}
	if result.Scopes[0].Items[1] != ".vimrc" {
		t.Errorf("items[1] = %q, want .vimrc", result.Scopes[0].Items[1])
	}
}

func TestList_CommonScope_ExplicitCommonHost(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	result, err := svc.List(context.Background(), "common", false)
	if err != nil {
		t.Fatalf("List with explicit common: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if result.Scopes[0].Name != "common" {
		t.Errorf("scope name = %q, want %q", result.Scopes[0].Name, "common")
	}
	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("items = %v, want [.bashrc]", result.Scopes[0].Items)
	}
}

func TestList_HostScope(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "testhost", ".bashrc", "# bashrc")

	result, err := svc.List(context.Background(), "testhost", false)
	if err != nil {
		t.Fatalf("List host scope: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if result.Scopes[0].Name != "testhost" {
		t.Errorf("scope name = %q, want %q", result.Scopes[0].Name, "testhost")
	}
	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("items = %v, want [.bashrc]", result.Scopes[0].Items)
	}
}

func TestList_EmptyScope(t *testing.T) {
	svc, _ := testhelpers.TestHome(t)

	result, err := svc.List(context.Background(), "", false)
	if err != nil {
		t.Fatalf("List empty scope: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if len(result.Scopes[0].Items) != 0 {
		t.Errorf("expected no items in empty scope, got %v", result.Scopes[0].Items)
	}
}

func TestList_All_OrderAndContents(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")
	setupTrackedFile(t, repoPath, home, "zhost", ".zshrc", "# zsh")
	setupTrackedFile(t, repoPath, home, "ahost", ".vimrc", "\" vim")

	result, err := svc.List(context.Background(), "", true)
	if err != nil {
		t.Fatalf("List --all: %v", err)
	}

	// Expect 3 scopes: common first, then hosts alphabetically.
	if len(result.Scopes) != 3 {
		t.Fatalf("expected 3 scopes, got %d: %v", len(result.Scopes), scopeNames(result))
	}

	if result.Scopes[0].Name != "common" {
		t.Errorf("scopes[0] = %q, want %q", result.Scopes[0].Name, "common")
	}
	if result.Scopes[1].Name != "ahost" {
		t.Errorf("scopes[1] = %q, want %q", result.Scopes[1].Name, "ahost")
	}
	if result.Scopes[2].Name != "zhost" {
		t.Errorf("scopes[2] = %q, want %q", result.Scopes[2].Name, "zhost")
	}

	// Verify contents of each scope.
	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("common items = %v, want [.bashrc]", result.Scopes[0].Items)
	}
	if len(result.Scopes[1].Items) != 1 || result.Scopes[1].Items[0] != ".vimrc" {
		t.Errorf("ahost items = %v, want [.vimrc]", result.Scopes[1].Items)
	}
	if len(result.Scopes[2].Items) != 1 || result.Scopes[2].Items[0] != ".zshrc" {
		t.Errorf("zhost items = %v, want [.zshrc]", result.Scopes[2].Items)
	}
}

func TestList_All_WithHostFlagErrors(t *testing.T) {
	svc, _ := testhelpers.TestHome(t)

	_, err := svc.List(context.Background(), "testhost", true)
	if err == nil {
		t.Fatal("expected error when --all and --host are combined, got nil")
	}
}

func TestList_All_EmptyScopes(t *testing.T) {
	svc, _ := testhelpers.TestHome(t)

	result, err := svc.List(context.Background(), "", true)
	if err != nil {
		t.Fatalf("List --all empty: %v", err)
	}

	// With no files tracked, only common should appear.
	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope (common), got %d: %v", len(result.Scopes), scopeNames(result))
	}
	if result.Scopes[0].Name != "common" {
		t.Errorf("scope name = %q, want common", result.Scopes[0].Name)
	}
	if len(result.Scopes[0].Items) != 0 {
		t.Errorf("expected no items, got %v", result.Scopes[0].Items)
	}
}

func TestList_NestedFile(t *testing.T) {
	svc, home := testhelpers.TestHome(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".config/git/config", "[user]")

	result, err := svc.List(context.Background(), "", false)
	if err != nil {
		t.Fatalf("List nested: %v", err)
	}

	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".config/git/config" {
		t.Errorf("items = %v, want [.config/git/config]", result.Scopes[0].Items)
	}
}

func TestList_V1_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	result, err := svc.List(context.Background(), "", false)
	if err != nil {
		t.Fatalf("List v1: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("items = %v, want [.bashrc]", result.Scopes[0].Items)
	}
}

func TestList_V1Legacy_CommonScope(t *testing.T) {
	svc, home := testhelpers.TestHomeV1Legacy(t)
	repoPath := svc.RepoPath()

	setupTrackedFile(t, repoPath, home, "common", ".bashrc", "# bashrc")

	result, err := svc.List(context.Background(), "", false)
	if err != nil {
		t.Fatalf("List v1 legacy: %v", err)
	}

	if len(result.Scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(result.Scopes))
	}
	if len(result.Scopes[0].Items) != 1 || result.Scopes[0].Items[0] != ".bashrc" {
		t.Errorf("items = %v, want [.bashrc]", result.Scopes[0].Items)
	}
}

func TestList_UninitializedRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	repoPath := filepath.Join(home, ".config", "lnk")
	svc := service.New(repoPath)

	_, err := svc.List(context.Background(), "", false)
	if err == nil {
		t.Fatal("expected error for uninitialized repo, got nil")
	}
}

// scopeNames extracts scope names from a ListResult for use in failure messages.
func scopeNames(result service.ListResult) []string {
	names := make([]string, len(result.Scopes))
	for i, s := range result.Scopes {
		names[i] = s.Name
	}
	return names
}
