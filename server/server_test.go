package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type testRepository string

func (r testRepository) Path() string { return string(r) }

type testStore struct {
	repo Repository
	err  error
	name string
}

func (s *testStore) Open(_ context.Context, name string) (Repository, error) {
	s.name = name
	return s.repo, s.err
}

func TestServerRoutePrefixStaticFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	store := &testStore{repo: testRepository(dir)}
	srv := New(Config{RoutePrefix: "/git"}, store)
	req := httptest.NewRequest(http.MethodGet, "/git/example.git/HEAD", nil)
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", res.Code, res.Body.String())
	}
	if store.name != "example.git" {
		t.Fatalf("expected repository example.git, got %q", store.name)
	}
	if res.Body.String() != "ref: refs/heads/main\n" {
		t.Fatalf("unexpected response body %q", res.Body.String())
	}
}

func TestServerStoreNotFound(t *testing.T) {
	srv := New(Config{}, &testStore{err: ErrRepositoryNotFound})
	req := httptest.NewRequest(http.MethodGet, "/missing.git/HEAD", nil)
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestServerStoreError(t *testing.T) {
	srv := New(Config{}, &testStore{err: errors.New("storage unavailable")})
	req := httptest.NewRequest(http.MethodGet, "/example.git/HEAD", nil)
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", res.Code)
	}
}

func TestServerRequireAuthRejectsWrongCredentials(t *testing.T) {
	store := &testStore{repo: testRepository(t.TempDir())}
	srv := New(Config{
		RequireAuth:    true,
		AuthUserEnvVar: "user",
		AuthPassEnvVar: "pass",
		UploadPack:     true,
	}, store)
	req := httptest.NewRequest(http.MethodGet, "/example.git/info/refs?service=git-upload-pack", nil)
	req.SetBasicAuth("wrong", "credentials")
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}
