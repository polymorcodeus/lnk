package service

import (
	"context"
	"fmt"

	gitpkg "github.com/polymorcodeus/lnk/internal/git"
)

// Commit stages all repo changes and creates a commit.
func (s *Service) Commit(ctx context.Context, message string) error {
	if err := s.requireGitRepo(); err != nil {
		return err
	}
	if err := s.git.EnsureGitConfigOnce(&s.gitConfigured); err != nil {
		return err
	}
	hasChanges, err := s.git.HasChanges()
	if err != nil {
		return err
	}
	if !hasChanges {
		return fmt.Errorf("no changes to commit")
	}
	if err := s.git.AddAll(); err != nil {
		return err
	}
	return s.commit(message)
}

// Push pushes existing commits only.
func (s *Service) Push(ctx context.Context) error {
	if err := s.requireGitRepo(); err != nil {
		return err
	}
	hasChanges, err := s.git.HasChanges()
	if err != nil {
		return err
	}
	if hasChanges {
		return fmt.Errorf("working tree is dirty; run 'lnk commit' or commit manually before push")
	}
	return s.git.Push()
}

// Pull updates the repo only.
func (s *Service) Pull(ctx context.Context) error {
	if err := s.requireGitRepo(); err != nil {
		return err
	}
	return s.git.Pull()
}

// Update pulls repo changes and then restores the effective machine profile.
func (s *Service) Update(ctx context.Context, host string) (RestoreInfo, error) {
	if err := s.Pull(ctx); err != nil {
		return RestoreInfo{}, err
	}
	return s.Restore(ctx, host, false)
}

// Status returns repo status information.
func (s *Service) Status(ctx context.Context) (*gitpkg.StatusInfo, error) {
	if err := s.requireGitRepo(); err != nil {
		return nil, err
	}
	return s.git.GetStatus()
}

// Diff returns the uncommitted repo diff.
func (s *Service) Diff(ctx context.Context) (string, error) {
	if err := s.requireGitRepo(); err != nil {
		return "", err
	}
	return s.git.Diff()
}
