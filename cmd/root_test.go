package cmd_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polymorcodeus/lnk/cmd"
	"github.com/polymorcodeus/lnk/service"
)

func TestPrintRestore(t *testing.T) {
	t.Run("live restore without backups", func(t *testing.T) {
		var buf bytes.Buffer
		info := service.RestoreInfo{
			Restored: []string{"a", "b"},
		}
		if err := cmd.PrintRestore(&buf, info, false); err != nil {
			t.Fatal(err)
		}

		out := buf.String()
		if !strings.Contains(out, "Restored 2 path(s)") {
			t.Errorf("unexpected header: %s", out)
		}
		if !strings.Contains(out, "  a\n") || !strings.Contains(out, "  b\n") {
			t.Errorf("missing paths: %s", out)
		}
		if strings.Contains(out, "Backed up") {
			t.Error("unexpected backup section")
		}
	})

	t.Run("dry run with backups", func(t *testing.T) {
		var buf bytes.Buffer
		info := service.RestoreInfo{
			Restored: []string{"a"},
			BackedUp: []string{"b", "c"},
		}
		if err := cmd.PrintRestore(&buf, info, true); err != nil {
			t.Fatal(err)
		}

		out := buf.String()
		if !strings.Contains(out, "Would restore 1 path(s)") {
			t.Errorf("unexpected header: %s", out)
		}
		if !strings.Contains(out, "Would back up 2 conflicting path(s)") {
			t.Errorf("unexpected backup header: %s", out)
		}
		if !strings.Contains(out, "  b\n") || !strings.Contains(out, "  c\n") {
			t.Errorf("missing backup paths: %s", out)
		}
	})
}

func TestPrintDoctor(t *testing.T) {
	t.Run("minimal report", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode: "check",
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Mode: check") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})

	t.Run("marker missing", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:          "check",
			MarkerMissing: true,
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Repo marker missing") {
			t.Error("expected marker missing message")
		}
	})

	t.Run("marker fixed", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:          "fix",
			MarkerMissing: true,
			MarkerFixed:   true,
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Repo marker was added") {
			t.Error("expected marker fixed message")
		}
	})

	t.Run("collisions", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode: "check",
			Collisions: []service.OwnershipCollision{
				{Path: "foo", Scopes: []string{"common", "host1"}},
			},
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "Ownership collisions:") {
			t.Error("expected collisions header")
		}
		if !strings.Contains(out, "  foo -> common, host1") {
			t.Errorf("unexpected collision line: %s", out)
		}
	})

	t.Run("empty scopes", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:        "check",
			EmptyScopes: []string{"host1", "host2"},
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Empty host scopes:") {
			t.Error("expected empty scopes header")
		}
	})

	t.Run("pruned scopes", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:         "fix",
			PrunedScopes: []string{"host1"},
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Pruned empty host scopes:") {
			t.Error("expected pruned scopes header")
		}
	})

	t.Run("broken symlink skipped", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:                    "check",
			BrokenSymlinkFixSkipped: true,
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Broken symlink repair was skipped") {
			t.Error("expected skipped message")
		}
	})

	t.Run("broken symlink fixed", func(t *testing.T) {
		var buf bytes.Buffer
		report := service.DoctorReport{
			Mode:             "fix",
			BrokenSymlinkFix: true,
		}
		if err := cmd.PrintDoctor(&buf, report); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "Broken symlinks repaired") {
			t.Error("expected fixed message")
		}
	})
}

func TestNewRootCommand(t *testing.T) {
	t.Run("version includes build info", func(t *testing.T) {
		cmd.SetVersion("1.0.0", "2024-01-01")
		root := cmd.NewRootCommand()
		if !strings.Contains(root.Version, "1.0.0") {
			t.Errorf("version missing tag: %s", root.Version)
		}
		if !strings.Contains(root.Version, "2024-01-01") {
			t.Errorf("version missing build time: %s", root.Version)
		}
	})

	t.Run("all subcommands registered", func(t *testing.T) {
		root := cmd.NewRootCommand()
		want := []string{
			"init", "clone", "add", "move", "remove", "forget",
			"list", "status", "diff", "commit", "push", "pull",
			"restore", "update", "doctor", "format", "bootstrap",
		}
		for _, name := range want {
			c, _, err := root.Find([]string{name})
			if err != nil {
				t.Fatalf("Find %q: %v", name, err)
			}
			if c == root {
				t.Errorf("subcommand %q not found", name)
			}
		}
	})

	t.Run("move flags are mutually exclusive", func(t *testing.T) {
		root := cmd.NewRootCommand()
		root.SetArgs([]string{"move", "foo", "--to-common", "--to-host", "h"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected error for mutually exclusive flags")
		}

		// Cobra's mutual exclusion message varies by version; check for key terms
		msg := err.Error()
		if !strings.Contains(msg, "to-common") && !strings.Contains(msg, "to-host") && !strings.Contains(msg, "mutually exclusive") {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("list flags are mutually exclusive", func(t *testing.T) {
		root := cmd.NewRootCommand()
		root.SetArgs([]string{"list", "--all", "--host", "h"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected error for mutually exclusive flags")
		}

		// Cobra's mutual exclusion message varies by version; check for key terms
		msg := err.Error()
		if !strings.Contains(msg, "all") && !strings.Contains(msg, "host") && !strings.Contains(msg, "mutually exclusive") {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("doctor flags are mutually exclusive", func(t *testing.T) {
		root := cmd.NewRootCommand()
		root.SetArgs([]string{"doctor", "--all", "--host", "h"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected error for mutually exclusive flags")
		}

		// Cobra's mutual exclusion message varies by version; check for key terms
		msg := err.Error()
		if !strings.Contains(msg, "all") && !strings.Contains(msg, "host") && !strings.Contains(msg, "mutually exclusive") {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("list flags are mutually exclusive", func(t *testing.T) {
		root := cmd.NewRootCommand()
		root.SetArgs([]string{"format", "--v1", "--v2"})

		err := root.Execute()
		if err == nil {
			t.Fatal("expected error for mutually exclusive flags")
		}

		// Cobra's mutual exclusion message varies by version; check for key terms
		msg := err.Error()
		if !strings.Contains(msg, "v1") && !strings.Contains(msg, "v2") && !strings.Contains(msg, "mutually exclusive") {
			t.Errorf("unexpected error message: %s", msg)
		}
	})
}
func TestInitCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	repoPath := filepath.Join(home, ".config", "lnk")

	var buf bytes.Buffer
	root := cmd.NewRootCommand()
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--repo", repoPath, "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput: %s", err, buf.String())
	}

	if !strings.Contains(buf.String(), "Initialized repo at") {
		t.Errorf("unexpected output: %s", buf.String())
	}
}
