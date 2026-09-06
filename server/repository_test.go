package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestCreateRepositoryInitializesBareRepository(t *testing.T) {
	ctx := context.Background()
	store := NewFilesystemStore(t.TempDir())
	srv := New(Config{}, store)

	repo, err := srv.CreateRepository(ctx, "team/example.git")
	if err != nil {
		t.Fatal(err)
	}

	opened, err := git.PlainOpen(repo.Path())
	if err != nil {
		t.Fatalf("open created repository: %v", err)
	}
	if opened == nil {
		t.Fatal("expected created repository")
	}

	if _, err := os.Stat(filepath.Join(repo.Path(), "HEAD")); err != nil {
		t.Fatalf("expected bare repository HEAD: %v", err)
	}
}

type failingCreateStore struct {
	path    string
	deleted bool
}

func (s *failingCreateStore) Open(context.Context, string) (Repository, error) {
	return nil, ErrRepositoryNotFound
}

func (s *failingCreateStore) Create(context.Context, string) (Repository, error) {
	return testRepository(s.path), nil
}

func (s *failingCreateStore) Delete(context.Context, string) error {
	s.deleted = true
	return nil
}

func (s *failingCreateStore) Exists(context.Context, string) (bool, error) { return false, nil }
func (s *failingCreateStore) List(context.Context) ([]string, error)       { return nil, nil }

func TestCreateRepositoryCleansUpOnInitFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	store := &failingCreateStore{path: path}
	srv := New(Config{}, store)

	_, err := srv.CreateRepository(context.Background(), "example.git")
	if err == nil {
		t.Fatal("expected repository initialization to fail")
	}
	if !store.deleted {
		t.Fatal("expected allocated repository to be deleted after init failure")
	}
}

func TestCreateRepositoryPreservesStoreError(t *testing.T) {
	want := errors.New("storage unavailable")
	store := &repositoryCreateErrorStore{err: want}
	srv := New(Config{}, store)

	_, err := srv.CreateRepository(context.Background(), "example.git")
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

type repositoryCreateErrorStore struct{ err error }

func (s *repositoryCreateErrorStore) Open(context.Context, string) (Repository, error) {
	return nil, ErrRepositoryNotFound
}
func (s *repositoryCreateErrorStore) Create(context.Context, string) (Repository, error) {
	return nil, s.err
}
func (s *repositoryCreateErrorStore) Delete(context.Context, string) error         { return nil }
func (s *repositoryCreateErrorStore) Exists(context.Context, string) (bool, error) { return false, nil }
func (s *repositoryCreateErrorStore) List(context.Context) ([]string, error)       { return nil, nil }
