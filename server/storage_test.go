package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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

func TestFilesystemStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewFilesystemStore(t.TempDir())

	repo, err := store.Create(ctx, "team/example.git")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(repo.Path()) != "example.git" {
		t.Fatalf("unexpected repository path %q", repo.Path())
	}

	exists, err := store.Exists(ctx, "team/example.git")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected repository to exist")
	}

	if _, err := store.Create(ctx, "team/example.git"); !errors.Is(err, ErrRepositoryExists) {
		t.Fatalf("expected ErrRepositoryExists, got %v", err)
	}

	repos, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"team/example.git"}; !reflect.DeepEqual(repos, want) {
		t.Fatalf("expected %v, got %v", want, repos)
	}

	if err := store.Delete(ctx, "team/example.git"); err != nil {
		t.Fatal(err)
	}

	exists, err = store.Exists(ctx, "team/example.git")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("expected repository to be deleted")
	}

	if err := store.Delete(ctx, "team/example.git"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected ErrRepositoryNotFound, got %v", err)
	}
}

func TestFilesystemStoreListSorted(t *testing.T) {
	ctx := context.Background()
	store := NewFilesystemStore(t.TempDir())
	for _, name := range []string{"z.git", "group/b.git", "a.git"} {
		if _, err := store.Create(ctx, name); err != nil {
			t.Fatal(err)
		}
	}

	repos, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.git", "group/b.git", "z.git"}
	if !reflect.DeepEqual(repos, want) {
		t.Fatalf("expected %v, got %v", want, repos)
	}
}

func TestFilesystemStoreRejectsTraversal(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	if _, err := store.Create(context.Background(), "../outside.git"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected traversal to be rejected, got %v", err)
	}
}

func TestFilesystemStoreCreateRequiresGitSuffix(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	if _, err := store.Create(context.Background(), "plain"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected suffixless repository name to be rejected, got %v", err)
	}
}

func TestFilesystemStoreRejectsSymlinkComponents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "team")); err != nil {
		t.Fatal(err)
	}

	store := NewFilesystemStore(root)
	if _, err := store.Create(context.Background(), "team/example.git"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("expected symlinked path to be rejected, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "example.git")); !os.IsNotExist(err) {
		t.Fatalf("repository escaped store root: %v", err)
	}
}
