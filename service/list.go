package service

import (
	"context"
	"fmt"

	"github.com/polymorcodeus/lnk/internal/tracker"
)

// List returns tracked items for common, one host, or all storage scopes.
func (s *Service) List(ctx context.Context, host string, all bool) (ListResult, error) {
	if err := s.requireGitRepo(); err != nil {
		return ListResult{}, err
	}
	if all && host != "" {
		return ListResult{}, fmt.Errorf("--host and --all cannot be combined")
	}

	var scopes []string
	var err error

	if all {
		scopes, err = s.hosts()
		if err != nil {
			return ListResult{}, err
		}
	} else {
		host = NormalizeHost(host)
		scopes = []string{host}
	}

	format, err := s.getFormat()
	if err != nil {
		return ListResult{}, err
	}
	result := ListResult{Scopes: make([]ScopeList, 0, len(scopes))}
	for _, scope := range scopes {
		items, err := tracker.New(s.repoPath, scope, format).GetManagedItems()
		if err != nil {
			return ListResult{}, err
		}
		result.Scopes = append(result.Scopes, ScopeList{
			Name:  scope,
			Items: items,
		})
	}
	return result, nil
}
