package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

var ErrRepositoryNotFound = errors.New("repository not found")

type Repository interface {
	Path() string
}

type RepositoryStore interface {
	Open(context.Context, string) (Repository, error)
}

type filesystemRepository struct {
	path string
}

func (r filesystemRepository) Path() string {
	return r.path
}

type FilesystemStore struct {
	Root string
}

func NewFilesystemStore(root string) *FilesystemStore {
	return &FilesystemStore{Root: root}
}

func (s *FilesystemStore) Open(_ context.Context, name string) (Repository, error) {
	root := s.Root
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = cwd
	}

	p := filepath.Join(root, filepath.Clean("/"+name))
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRepositoryNotFound
		}
		return nil, err
	}

	return filesystemRepository{path: p}, nil
}
