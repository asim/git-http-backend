package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticationProtectsAllRepositoryEndpoints(t *testing.T) {
	endpoints := []struct{ method, path string }{
		{"GET", "/example.git/info/refs?service=git-upload-pack"},
		{"GET", "/example.git/info/refs?service=git-receive-pack"},
		{"POST", "/example.git/git-upload-pack"},
		{"POST", "/example.git/git-receive-pack"},
		{"GET", "/example.git/info/refs"},
		{"GET", "/example.git/HEAD"},
		{"GET", "/example.git/objects/info/alternates"},
		{"GET", "/example.git/objects/info/http-alternates"},
		{"GET", "/example.git/objects/info/packs"},
		{"GET", "/example.git/objects/info/commit-graph"},
		{"GET", "/example.git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"GET", "/example.git/objects/pack/pack-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.pack"},
		{"GET", "/example.git/objects/pack/pack-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.idx"},
	}
	for _, endpoint := range endpoints {
		for _, credentials := range []string{"missing", "wrong-user", "wrong-password", "malformed"} {
			t.Run(endpoint.method+endpoint.path+"/"+credentials, func(t *testing.T) {
				store := &testStore{err: ErrRepositoryNotFound}
				config := DefaultConfig
				config.RequireAuth = true
				config.AuthUserEnvVar = "user"
				config.AuthPassEnvVar = "pass"
				config.RoutePrefix = "/git"
				srv := New(config, store)
				req := httptest.NewRequest(endpoint.method, "/git"+endpoint.path, nil)
				switch credentials {
				case "wrong-user":
					req.SetBasicAuth("wrong", "pass")
				case "wrong-password":
					req.SetBasicAuth("user", "wrong")
				case "malformed":
					req.Header.Set("Authorization", "Basic invalid")
				}
				if endpoint.method == "POST" {
					rpc := "upload-pack"
					if endpoint.path == "/example.git/git-receive-pack" {
						rpc = "receive-pack"
					}
					req.Header.Set("Content-Type", "application/x-git-"+rpc+"-request")
				}
				res := httptest.NewRecorder()
				srv.ServeHTTP(res, req)
				if res.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, want 401", res.Code)
				}
				if got := res.Header().Get("WWW-Authenticate"); got != `Basic realm="authorization needed"` {
					t.Fatalf("unexpected challenge %q", got)
				}
				if store.name != "" {
					t.Fatalf("unauthenticated request opened repository %q", store.name)
				}
			})
		}
	}
}

func TestStaticRepositoryAuthentication(t *testing.T) {
	dir := t.TempDir()
	const head = "ref: refs/heads/main\n"
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(head), 0600); err != nil {
		t.Fatal(err)
	}
	for _, requireAuth := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "authenticated"}[requireAuth], func(t *testing.T) {
			config := DefaultConfig
			config.RequireAuth = requireAuth
			config.AuthUserEnvVar = "user"
			config.AuthPassEnvVar = "pass"
			req := httptest.NewRequest("GET", "/example.git/HEAD", nil)
			if requireAuth {
				req.SetBasicAuth("user", "pass")
			}
			res := httptest.NewRecorder()
			New(config, &testStore{repo: testRepository(dir)}).ServeHTTP(res, req)
			if res.Code != http.StatusOK || res.Body.String() != head {
				t.Fatalf("response = %d %q", res.Code, res.Body.String())
			}
		})
	}
}
