package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemStoreOpen(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "example.git")
	if err := os.Mkdir(repoPath, 0755); err != nil {
		t.Fatal(err)
	}

	store := NewFilesystemStore(root)
	repo, err := store.Open(context.Background(), "example.git")
	if err != nil {
		t.Fatal(err)
	}
	if repo.Path() != repoPath {
		t.Fatalf("expected %q, got %q", repoPath, repo.Path())
	}
}

func TestFilesystemStoreNotFound(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	_, err := store.Open(context.Background(), "missing.git")
	if !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected ErrRepositoryNotFound, got %v", err)
	}
}
