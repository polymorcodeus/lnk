# Testing Guide

This document describes how to run and extend the `lnk` test suite.

## Quick Start

Run the full quality gate:

```bash
make check
```

This runs formatting, vet, lint, unit tests, and integration tests.

## Test Layers

### Unit Tests

Unit tests live next to the code they exercise (e.g., `service/add_test.go`) and use the standard `testing` package.

```bash
make test
# or verbose
make test-v
```

Unit tests isolate the service and leaf packages with the helpers in `internal/testhelpers`. They do not require network access.

### Integration Tests

Integration tests live in `tests/integration/` and are guarded by the `integration` build tag.

```bash
make test-integration
# or directly
go test -tags integration ./tests/integration/
```

These tests simulate full user workflows: init, add, restore, update, doctor, and format migration across common and host scopes.

## Scope Test Matrix

`lnk` distinguishes between the `common` scope, per-machine host scopes, and project scopes. When adding tests for any command that accepts a `--host` flag, exercise both dimensions. When adding tests for project scope commands, set up a git repository with an `origin` remote.

| Scenario | Host argument | Storage directory | Tracker file |
| --- | --- | --- | --- |
| Common scope (default) | `""` or `"common"` | `common.lnk/` (v2) or repo root (v1) | `.lnk.common` (v2) or `.lnk` (v1) |
| Host scope | `"work"`, `"laptop"`, etc. | `<host>.lnk/` | `.lnk.<host>` |
| Project scope | N/A (uses `--dir`) | `projects/<normalized-origin>/` | N/A (uses `.lnkinclude` patterns) |

Use the helpers below to set up each scope consistently:

- `testhelpers.TestHome(t)` - temp `$HOME` with a fresh v2 repo
- `testhelpers.TestHomeV1(t)` - v2 repo marker but v1 storage layout
- `testhelpers.TestHomeV1Legacy(t)` - v1 repo without a `.lnkrepo` marker
- `testhelpers.InitGitRepo(t, dir)` - initialize a git repo with test config
- `testhelpers.NewBareRemote(t)` - create a bare repo for push/pull tests
- `setupTrackedFile(t, repoPath, home, scope, relativePath, content)` - creates storage, symlink, and tracker entry for a scope

For project scope tests, also use `resolver.ResolveProjectID(ctx, projectRoot)` to compute the expected storage directory under `projects/`.

## Key Edge Cases

When adding coverage, consider these regression-sensitive scenarios:

- **Symlink already exists**: `lnk add` rejects symlinks because it cannot manage them.
- **Backup collision**: `lnk restore`, `lnk doctor --fix`, and `lnk project restore` refuse to overwrite an existing `<path>.lnk-backup` file.
- **Dry-run behavior**: `lnk restore --dry-run` and `lnk project restore --dry-run` report what would happen without creating symlinks, backups, or removing files.
- **Dirty tree**: `lnk doctor --fix` refuses to run when the working tree has uncommitted changes.
- **Uninitialized repo**: commands that require a repo return `ErrNotInitialized`.
- **Project scope requires git repo**: `lnk project` commands fail with `ErrOutsideGitRepo` when run outside a git repository.
- **Project scope requires origin**: `lnk project` commands fail with `resolver.ErrNoOrigin` when the project git repo has no `origin` remote.
- **No project patterns**: `lnk project push` returns `ErrNoPatterns` when no global or local `.lnkinclude` patterns exist.
- **Project `.git` skipped**: `lnk project push` skips any `.git` directory while walking the project tree.
- **Already symlinked project files**: `lnk project push` skips files that already point to the correct storage path.

## Coverage

Generate an HTML coverage report:

```bash
make test-cover
open coverage.html
```

## Writing New Tests

- Prefer table-driven tests with named subtests.
- Use `errors.Is` to assert sentinel errors; do not match on error strings.
- Clean state with `t.TempDir()` and `testhelpers.TestHome(t)` instead of touching the real `$HOME`.
- Integration tests must include `//go:build integration` at the top of the file.
