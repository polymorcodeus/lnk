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

`lnk` distinguishes between the `common` scope and per-machine host scopes. When adding tests for any command that accepts a `--host` flag, exercise both dimensions:

| Scenario | Host argument | Storage directory | Tracker file |
| --- | --- | --- | --- |
| Common scope (default) | `""` or `"common"` | `common.lnk/` (v2) or repo root (v1) | `.lnk.common` (v2) or `.lnk` (v1) |
| Host scope | `"work"`, `"laptop"`, etc. | `<host>.lnk/` | `.lnk.<host>` |

Use the helpers below to set up each scope consistently:

- `testhelpers.TestHome(t)` - temp `$HOME` with a fresh v2 repo
- `testhelpers.TestHomeV1(t)` - v2 repo marker but v1 storage layout
- `testhelpers.TestHomeV1Legacy(t)` - v1 repo without a `.lnkrepo` marker
- `setupTrackedFile(t, repoPath, home, scope, relativePath, content)` - creates storage, symlink, and tracker entry for a scope

## Key Edge Cases

When adding coverage, consider these regression-sensitive scenarios:

- **Symlink already exists**: `lnk add` rejects symlinks because it cannot manage them.
- **Backup collision**: `lnk restore` and `lnk doctor --fix` refuse to overwrite an existing `<path>.lnk-backup` file.
- **Dry-run behavior**: `lnk restore --dry-run` reports what would happen without creating symlinks, backups, or removing files.
- **Dirty tree**: `lnk doctor --fix` refuses to run when the working tree has uncommitted changes.
- **Uninitialized repo**: commands that require a repo return `ErrNotInitialized`.

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
