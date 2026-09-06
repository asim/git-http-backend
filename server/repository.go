package server

import (
	"context"
	"fmt"

	git "github.com/go-git/go-git/v5"
)

// CreateRepository allocates repository storage and initializes it as a bare Git repository.
func (s *Server) CreateRepository(ctx context.Context, name string) (Repository, error) {
	repo, err := s.Store.Create(ctx, name)
	if err != nil {
		return nil, err
	}

	if _, err := git.PlainInit(repo.Path(), true); err != nil {
		if cleanupErr := s.Store.Delete(context.Background(), name); cleanupErr != nil {
			return nil, fmt.Errorf("initialize bare repository: %w; cleanup failed: %v", err, cleanupErr)
		}
		return nil, fmt.Errorf("initialize bare repository: %w", err)
	}

	return repo, nil
}
