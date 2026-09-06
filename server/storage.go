package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

var (
	ErrRepositoryNotFound = errors.New("repository not found")
	ErrRepositoryExists   = errors.New("repository already exists")
)

type Repository interface {
	Path() string
}

type Store interface {
	Open(context.Context, string) (Repository, error)
	Create(context.Context, string) (Repository, error)
	Delete(context.Context, string) error
	Exists(context.Context, string) (bool, error)
	List(context.Context) ([]string, error)
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
	p, err := s.repositoryPath(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRepositoryNotFound
		}
		return nil, err
	}

	return filesystemRepository{path: p}, nil
}

func (s *FilesystemStore) Create(_ context.Context, name string) (Repository, error) {
	p, err := s.repositoryPath(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(p); err == nil {
		return nil, ErrRepositoryExists
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(p, 0755); err != nil {
		if os.IsExist(err) {
			return nil, ErrRepositoryExists
		}
		return nil, err
	}

	return filesystemRepository{path: p}, nil
}

func (s *FilesystemStore) Delete(_ context.Context, name string) error {
	p, err := s.repositoryPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return ErrRepositoryNotFound
		}
		return err
	}
	return os.RemoveAll(p)
}

func (s *FilesystemStore) Exists(_ context.Context, name string) (bool, error) {
	p, err := s.repositoryPath(name)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *FilesystemStore) List(_ context.Context) ([]string, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}

	var repos []string
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == root {
				return nil
			}
			return err
		}
		if !info.IsDir() || p == root {
			return nil
		}
		if filepath.Ext(info.Name()) != ".git" {
			return nil
		}
		name, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		repos = append(repos, filepath.ToSlash(name))
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(repos)
	return repos, nil
}

func (s *FilesystemStore) root() (string, error) {
	if s.Root != "" {
		return filepath.Abs(s.Root)
	}
	return os.Getwd()
}

func (s *FilesystemStore) repositoryPath(name string) (string, error) {
	root, err := s.root()
	if err != nil {
		return "", err
	}

	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || len(clean) >= 3 && clean[:3] == ".."+string(filepath.Separator) {
		return "", ErrRepositoryNotFound
	}

	return filepath.Join(root, clean), nil
}
