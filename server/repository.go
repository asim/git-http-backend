package server

import (
	"context"
	"fmt"
	"os/exec"
)

// CreateRepository allocates repository storage and initializes it as a bare Git repository.
func (s *Server) CreateRepository(ctx context.Context, name string) (Repository, error) {
	repo, err := s.Store.Create(ctx, name)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, s.Config.GitBinPath, "init", "--bare", repo.Path())
	s.Config.CommandFunc(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		if cleanupErr := s.Store.Delete(context.Background(), name); cleanupErr != nil {
			return nil, fmt.Errorf("git init --bare failed: %w: %s; cleanup failed: %v", err, output, cleanupErr)
		}
		return nil, fmt.Errorf("git init --bare failed: %w: %s", err, output)
	}

	return repo, nil
}
