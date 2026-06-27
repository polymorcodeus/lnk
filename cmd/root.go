// Package cmd implements the v2 CLI.
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/polymorcodeus/lnk/service"
)

var (
	version   = "internal"
	buildTime = "unknown"
)

// SetVersion sets the build-time version and build timestamp injected by ldflags.
func SetVersion(v, bt string) {
	version = v
	buildTime = bt
}

// NewRootCommand constructs the v2 root command.
func NewRootCommand() *cobra.Command {
	var repoPath string

	rootCmd := &cobra.Command{
		Use:           "lnk",
		Short:         "Git-native dotfiles management, v2",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       fmt.Sprintf("%s (built %s)", version, buildTime),
	}

	rootCmd.PersistentFlags().StringVar(&repoPath, "repo", "", "path to the lnk repository")

	rootCmd.AddCommand(newInitCmd(&repoPath))
	rootCmd.AddCommand(newCloneCmd(&repoPath))
	rootCmd.AddCommand(newAddCmd(&repoPath))
	rootCmd.AddCommand(newMoveCmd(&repoPath))
	rootCmd.AddCommand(newRemoveCmd(&repoPath))
	rootCmd.AddCommand(newForgetCmd(&repoPath))
	rootCmd.AddCommand(newListCmd(&repoPath))
	rootCmd.AddCommand(newStatusCmd(&repoPath))
	rootCmd.AddCommand(newDiffCmd(&repoPath))
	rootCmd.AddCommand(newCommitCmd(&repoPath))
	rootCmd.AddCommand(newPushCmd(&repoPath))
	rootCmd.AddCommand(newPullCmd(&repoPath))
	rootCmd.AddCommand(newRestoreCmd(&repoPath))
	rootCmd.AddCommand(newUpdateCmd(&repoPath))
	rootCmd.AddCommand(newDoctorCmd(&repoPath))
	rootCmd.AddCommand(newBootstrapCmd(&repoPath))
	rootCmd.AddCommand(newFormatCmd(&repoPath))

	return rootCmd
}

// Execute runs the v2 CLI.
func Execute() {
	rootCmd := NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// svc constructs a Service for the resolved repo path.
// Call once per command invocation and reuse the result — construction
// reads the version marker from disk.
func svc(repoFlag *string, opts ...service.Option) *service.Service {
	resolvedRepo := service.ResolveRepoPath(strings.TrimSpace(*repoFlag))
	return service.NewBuilder(resolvedRepo, opts...)
}

// newInitCmd returns the "init" subcommand.
func newInitCmd(repoFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create or adopt a local lnk repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Init(cmd.Context()); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Initialized repo at %s\n", app.RepoPath())
			return err
		},
	}
}

// newCloneCmd returns the "clone" subcommand.
func newCloneCmd(repoFlag *string) *cobra.Command {
	var withBootstrap bool

	cmd := &cobra.Command{
		Use:   "clone <url>",
		Short: "Clone a remote lnk repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			ran, err := app.Clone(cmd.Context(), args[0], withBootstrap, cmd.OutOrStdout(), cmd.ErrOrStderr(), os.Stdin)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Cloned repo to %s\n", app.RepoPath()); err != nil {
				return err
			}
			if !withBootstrap {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Run 'lnk restore' or 'lnk update' when ready")
				return err
			}
			if ran {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Ran bootstrap.sh")
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "No bootstrap.sh found")
			return err
		},
	}

	cmd.Flags().BoolVar(&withBootstrap, "bootstrap", false, "run bootstrap.sh after clone")
	return cmd
}

// newAddCmd returns the "add" subcommand.
func newAddCmd(repoFlag *string) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "add [--host H] <path...>",
		Short: "Track one or more paths",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Add(cmd.Context(), host, args); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Tracked %d path(s) in %s scope\n", len(args), host)
			return err
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "track paths in a host-specific scope")
	return cmd
}

// newMoveCmd returns the "move" subcommand.
func newMoveCmd(repoFlag *string) *cobra.Command {
	var toCommon bool
	var toHost string

	cmd := &cobra.Command{
		Use:   "move <path> (--to-common | --to-host H)",
		Short: "Move a tracked path between scopes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Move(cmd.Context(), args[0], toHost, toCommon); err != nil {
				return err
			}
			target := "common"
			if toHost != "" {
				target = toHost
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Moved %s to %s scope\n", args[0], target)
			return err
		},
	}

	cmd.Flags().BoolVar(&toCommon, "to-common", false, "move the path into common scope")
	cmd.Flags().StringVar(&toHost, "to-host", "", "move the path into a host-specific scope")
	cmd.MarkFlagsMutuallyExclusive("to-common", "to-host")
	return cmd
}

// newRemoveCmd returns the "remove" subcommand.
func newRemoveCmd(repoFlag *string) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "remove [--host H] <path>",
		Short: "Stop managing a path and restore it locally",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Remove(cmd.Context(), host, args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from %s scope\n", args[0], host)
			return err
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "remove a host-scoped path")
	return cmd
}

// newForgetCmd returns the "forget" subcommand.
func newForgetCmd(repoFlag *string) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "forget [--host H] <path>",
		Short: "Stop managing a path but keep its stored repo copy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Forget(cmd.Context(), host, args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Forgot %s from %s scope\n", args[0], host)
			return err
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "forget a host-scoped path")
	return cmd
}

// newListCmd returns the "list" subcommand.
func newListCmd(repoFlag *string) *cobra.Command {
	var host string
	var all bool

	cmd := &cobra.Command{
		Use:   "list [--host H | --all]",
		Short: "List tracked paths by storage scope",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			result, err := app.List(cmd.Context(), host, all)
			if err != nil {
				return err
			}
			for i, scope := range result.Scopes {
				if i > 0 {
					if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", scope.Name); err != nil {
					return err
				}
				if len(scope.Items) == 0 {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  (no files)"); err != nil {
						return err
					}
					continue
				}
				for _, item := range scope.Items {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", item); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "list one host scope")
	cmd.Flags().BoolVar(&all, "all", false, "list common plus all host scopes")
	cmd.MarkFlagsMutuallyExclusive("all", "host")
	return cmd
}

// newStatusCmd returns the "status" subcommand.
func newStatusCmd(repoFlag *string) *cobra.Command {
	var color bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show repo sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag, service.WithColor(color))

			status, err := app.Status(cmd.Context())
			if err != nil {
				return err
			}
			writer := cmd.OutOrStdout()
			if status.Remote == "" {
				if _, err := fmt.Fprintln(writer, "Remote not set"); err != nil {
					return err
				}
			}
			// Print the full git status output
			if _, err := io.WriteString(writer, status.Output); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&color, "color", false, "set to display `git status` in color, default is no color")
	return cmd
}

// newDiffCmd returns the "diff" subcommand.
func newDiffCmd(repoFlag *string) *cobra.Command {
	var color bool

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show the uncommitted repo diff",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag, service.WithColor(color))

			diff, err := app.Diff(cmd.Context())
			if err != nil {
				return err
			}
			if diff == "" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "No uncommitted changes")
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), diff)
			return err
		},
	}

	cmd.Flags().BoolVar(&color, "color", false, "set to display `git diff` in color, default is no color")
	return cmd
}

// newCommitCmd returns the "commit" subcommand.
func newCommitCmd(repoFlag *string) *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Stage all repo changes and create a commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if message == "" {
				message = "lnk: sync configuration files"
			}
			if err := app.Commit(cmd.Context(), message); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Committed repo changes: %s\n", message)
			return err
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message")
	return cmd
}

// newPushCmd returns the "push" subcommand.
func newPushCmd(repoFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push existing commits only",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Push(cmd.Context()); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Pushed existing commits")
			return err
		},
	}
}

// newPullCmd returns the "pull" subcommand.
func newPullCmd(repoFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull repo changes only",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			if err := app.Pull(cmd.Context()); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Pulled repo changes")
			return err
		},
	}
}

// newRestoreCmd returns the "restore" subcommand.
func newRestoreCmd(repoFlag *string) *cobra.Command {
	var host string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "restore [--host H] [--dry-run]",
		Short: "Restore the effective machine profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			info, err := app.Restore(cmd.Context(), host, dryRun)
			if err != nil {
				return err
			}
			return printRestore(cmd.OutOrStdout(), info, dryRun)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "include one host scope in the restored profile")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview restore actions without changing files")
	return cmd
}

// newUpdateCmd returns the "update" subcommand.
func newUpdateCmd(repoFlag *string) *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "update [--host H]",
		Short: "Pull repo changes and restore the effective machine profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			info, err := app.Update(cmd.Context(), host)
			if err != nil {
				return err
			}
			if err := printRestore(cmd.OutOrStdout(), info, false); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Updated repo and machine state")
			return err
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "include one host scope in the restored profile")
	return cmd
}

// newDoctorCmd returns the "doctor" subcommand.
func newDoctorCmd(repoFlag *string) *cobra.Command {
	var host string
	var all bool
	var fix bool
	var pruneEmpty bool

	cmd := &cobra.Command{
		Use:   "doctor [--host H | --all] [--fix] [--prune-empty]",
		Short: "Audit repo and profile health",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			report, err := app.Doctor(cmd.Context(), host, all, fix, pruneEmpty)
			if err != nil {
				return err
			}
			return printDoctor(cmd.OutOrStdout(), report)
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "check one host profile")
	cmd.Flags().BoolVar(&all, "all", false, "check all storage scopes")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply safe automatic fixes")
	cmd.Flags().BoolVar(&pruneEmpty, "prune-empty", false, "remove empty host scopes and their storage directories when passed with --fix")
	cmd.MarkFlagsMutuallyExclusive("all", "host")
	return cmd
}

// newFormatCmd returns the "format" subcommand.
func newFormatCmd(repoFlag *string) *cobra.Command {
	var ver1 bool
	var ver2 bool

	cmd := &cobra.Command{
		Use:   "format [--v1 | --v2]",
		Short: "Update format of common lnks",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			result, err := app.Format(cmd.Context(), ver1, ver2)
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), result)
			return err
		},
	}

	cmd.Flags().BoolVar(&ver1, "v1", false, "legacy format, with dotfiles and folders in root directory")
	cmd.Flags().BoolVar(&ver2, "v2", false, "version2 format, common dotfiles aggregated under common.lnk")
	cmd.MarkFlagsMutuallyExclusive("v1", "v2")
	return cmd
}

// newBootstrapCmd returns the "bootstrap" subcommand.
func newBootstrapCmd(repoFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap",
		Short: "Run bootstrap.sh explicitly",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := svc(repoFlag)
			ran, err := app.Bootstrap(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), os.Stdin)
			if err != nil {
				return err
			}
			if !ran {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "No bootstrap.sh found")
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Ran bootstrap.sh")
			return err
		},
	}
}

// dryRunPrefix selects a prefix string based on whether dry-run mode is active.
func dryRunPrefix(dryRun bool, live, dry string) string {
	if dryRun {
		return dry
	}
	return live
}

// printRestore writes restore/update results to w.
func printRestore(w io.Writer, info service.RestoreInfo, dryRun bool) error {
	prefix := dryRunPrefix(dryRun, "Restored", "Would restore")
	backupPrefix := dryRunPrefix(dryRun, "Backed up", "Would back up")

	if _, err := fmt.Fprintf(w, "%s %d path(s)\n", prefix, len(info.Restored)); err != nil {
		return err
	}
	for _, path := range info.Restored {
		if _, err := fmt.Fprintf(w, "  %s\n", path); err != nil {
			return err
		}
	}
	if len(info.BackedUp) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s %d conflicting path(s)\n", backupPrefix, len(info.BackedUp)); err != nil {
		return err
	}
	for _, path := range info.BackedUp {
		if _, err := fmt.Fprintf(w, "  %s\n", path); err != nil {
			return err
		}
	}
	return nil
}

// printDoctor writes the doctor report to w.
func printDoctor(w io.Writer, report service.DoctorReport) error {
	if _, err := fmt.Fprintf(w, "Mode: %s\n", report.Mode); err != nil {
		return err
	}
	if report.MarkerMissing {
		line := "Repo marker missing"
		if report.MarkerFixed {
			line = "Repo marker was added"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if len(report.Collisions) > 0 {
		if _, err := fmt.Fprintln(w, "Ownership collisions:"); err != nil {
			return err
		}
		for _, collision := range report.Collisions {
			if _, err := fmt.Fprintf(w, "  %s -> %s\n", collision.Path, strings.Join(collision.Scopes, ", ")); err != nil {
				return err
			}
		}
	}
	for _, result := range report.ScopeResults {
		if err := result.Print(w); err != nil {
			return err
		}
	}
	if len(report.EmptyScopes) > 0 {
		if _, err := fmt.Fprintln(w, "\nEmpty host scopes:"); err != nil {
			return err
		}
		for _, scope := range report.EmptyScopes {
			if _, err := fmt.Fprintf(w, "  %s\n", scope); err != nil {
				return err
			}
		}
	}
	if len(report.PrunedScopes) > 0 {
		if _, err := fmt.Fprintln(w, "Pruned empty host scopes:"); err != nil {
			return err
		}
		for _, scope := range report.PrunedScopes {
			if _, err := fmt.Fprintf(w, "  %s\n", scope); err != nil {
				return err
			}
		}
	}
	if report.BrokenSymlinkFixSkipped {
		_, err := fmt.Fprintln(w, "Broken symlink repair was skipped in --all mode")
		return err
	}
	if report.BrokenSymlinkFix {
		_, err := fmt.Fprintln(w, "Broken symlinks repaired")
		return err
	}
	return nil
}
