package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if _, err := opened.Worktree(); !errors.Is(err, git.ErrIsBareRepository) {
		t.Fatalf("expected bare repository, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo.Path(), "HEAD")); err != nil {
		t.Fatalf("expected bare repository HEAD: %v", err)
	}
}

type failingCreateStore struct {
	path      string
	deleted   bool
	deleteErr error
}

func (s *failingCreateStore) Open(context.Context, string) (Repository, error) {
	return nil, ErrRepositoryNotFound
}

func (s *failingCreateStore) Create(context.Context, string) (Repository, error) {
	return testRepository(s.path), nil
}

func (s *failingCreateStore) Delete(context.Context, string) error {
	s.deleted = true
	return s.deleteErr
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

func TestCreateRepositoryNativeHTTPPushAndClone(t *testing.T) {
	for _, requireAuth := range []bool{false, true} {
		name := "public"
		if requireAuth {
			name = "authenticated"
		}
		t.Run(name, func(t *testing.T) { testNativeHTTPPushAndClone(t, requireAuth) })
	}
}

func testNativeHTTPPushAndClone(t *testing.T, requireAuth bool) {
	t.Helper()

	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("native git is required for HTTP integration test")
	}
	config := DefaultConfig
	config.GitBinPath = gitBin
	config.RequireAuth = requireAuth
	config.AuthUserEnvVar = "user"
	config.AuthPassEnvVar = "pass"
	srv := New(config, NewFilesystemStore(t.TempDir()))
	if _, err := srv.CreateRepository(context.Background(), "team/example.git"); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(srv)
	defer httpServer.Close()

	run := func(dir string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, gitBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	source := t.TempDir()
	run(source, "init")
	run(source, "symbolic-ref", "HEAD", "refs/heads/master")
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hello from native git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(source, "add", "hello.txt")
	run(source, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "initial commit")
	remoteURL, err := url.Parse(httpServer.URL + "/team/example.git")
	if err != nil {
		t.Fatal(err)
	}
	if requireAuth {
		remoteURL.User = url.UserPassword("user", "pass")
	}
	run(source, "push", remoteURL.String(), "master")
	clone := filepath.Join(t.TempDir(), "clone")
	run(source, "clone", remoteURL.String(), clone)
	if got, want := run(clone, "rev-parse", "HEAD"), run(source, "rev-parse", "HEAD"); got != want {
		t.Fatalf("cloned commit = %s, want %s", got, want)
	}
	contents, err := os.ReadFile(filepath.Join(clone, "hello.txt"))
	if err != nil || string(contents) != "hello from native git\n" {
		t.Fatalf("cloned file = %q, error = %v", contents, err)
	}
}

func TestCreateRepositoryReportsCleanupFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &failingCreateStore{path: path, deleteErr: errors.New("delete unavailable")}
	_, err := New(Config{}, store).CreateRepository(context.Background(), "example.git")
	if err == nil || !strings.Contains(err.Error(), "cleanup failed: delete unavailable") {
		t.Fatalf("expected initialization and cleanup failure, got %v", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("initialization error not preserved: %v", err)
	}
}

func TestCreateRepositoryDoesNotDeleteExistingRepository(t *testing.T) {
	store := NewFilesystemStore(t.TempDir())
	srv := New(Config{}, store)
	repo, err := srv.CreateRepository(context.Background(), "example.git")
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.CreateRepository(context.Background(), "example.git")
	if !errors.Is(err, ErrRepositoryExists) {
		t.Fatalf("expected existing repository error, got %v", err)
	}
	if _, err := git.PlainOpen(repo.Path()); err != nil {
		t.Fatalf("existing repository damaged: %v", err)
	}
}
