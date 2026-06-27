package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	fspkg "github.com/polymorcodeus/lnk/internal/fs"
)

// Restore applies the effective machine profile: common only, or common + host.
func (s *Service) Restore(ctx context.Context, host string, dryRun bool) (RestoreInfo, error) {
	if err := s.requireGitRepo(); err != nil {
		return RestoreInfo{}, err
	}
	collisions, err := s.scanCollisions()
	if err != nil {
		return RestoreInfo{}, err
	}
	if len(collisions) > 0 {
		return RestoreInfo{}, fmt.Errorf("restore blocked by duplicate ownership; run 'lnk doctor' first")
	}

	host = NormalizeHost(host)
	items, err := s.profileItems(host)
	if err != nil {
		return RestoreInfo{}, err
	}

	info := RestoreInfo{}
	fs := fspkg.New()
	for _, item := range items {
		if _, err := os.Stat(item.RepoPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if isManagedSymlink(item.LivePath, item.RepoPath) {
			continue
		}

		if currentInfo, err := os.Lstat(item.LivePath); err == nil {
			if currentInfo.Mode()&os.ModeSymlink == 0 {
				info.BackedUp = append(info.BackedUp, item.RelativePath)
				if !dryRun {
					backupPath := item.LivePath + ".lnk-backup"
					if _, err := os.Lstat(backupPath); err == nil {
						return RestoreInfo{}, fmt.Errorf("backup path already exists: %s", backupPath)
					}
					if err := os.Rename(item.LivePath, backupPath); err != nil {
						return RestoreInfo{}, fmt.Errorf("backup existing file %s: %w", item.LivePath, err)
					}
				}
			} else if !dryRun {
				if err := os.Remove(item.LivePath); err != nil {
					return RestoreInfo{}, fmt.Errorf("remove stale symlink %s: %w", item.LivePath, err)
				}
			}
		}

		info.Restored = append(info.Restored, item.RelativePath)
		if dryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.LivePath), 0o755); err != nil {
			return RestoreInfo{}, fmt.Errorf("create live parent directory: %w", err)
		}
		if err := fs.CreateSymlink(item.RepoPath, item.LivePath); err != nil {
			return RestoreInfo{}, err
		}
	}
	return info, nil
}
