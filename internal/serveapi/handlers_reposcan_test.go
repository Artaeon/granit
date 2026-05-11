package serveapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/reposcan"
	"github.com/artaeon/granit/internal/vault"
)

// reposcanTestServer plants a Server with a vault-root sandbox and
// wires only the reposcan route. Path-safety lives in the package
// (pinned by reposcan_test.go); these tests cover the wire shape
// + the HTTP-layer status mapping the frontend depends on.
func reposcanTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{Vault: v, Logger: slog.Default()}}
	r := chi.NewRouter()
	r.Post("/api/v1/reposcan", s.handleScanRepo)
	return s, r, root
}

func scanPost(t *testing.T, h http.Handler, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rdr = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reposcan", rdr)
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code, rr.Body.Bytes()
}

func TestHandleScanRepo_HappyPath(t *testing.T) {
	_, h, root := reposcanTestServer(t)
	repo := filepath.Join(root, "myproj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Myproj\n\nDoes a thing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := scanPost(t, h, map[string]string{"path": repo})
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, body)
	}
	var got reposcan.Context
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "myproj" {
		t.Errorf("Name = %q, want myproj", got.Name)
	}
	if got.ReadmeName != "README.md" {
		t.Errorf("ReadmeName = %q, want README.md", got.ReadmeName)
	}
}

func TestHandleScanRepo_RejectsEmptyPath(t *testing.T) {
	_, h, _ := reposcanTestServer(t)
	code, _ := scanPost(t, h, map[string]string{"path": ""})
	if code != http.StatusBadRequest {
		t.Errorf("empty path: status %d, want 400", code)
	}
}

func TestHandleScanRepo_RejectsMalformedJSON(t *testing.T) {
	_, h, _ := reposcanTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reposcan", bytes.NewReader([]byte("{ broken")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("malformed json: status %d, want 400", rr.Code)
	}
}

func TestHandleScanRepo_403OnOutsideAllowedRoots(t *testing.T) {
	// Scanning a tmp dir that's not under the vault root or home
	// must 403 — distinct from 404 so the UI can surface a clear
	// "this lives outside your home / vault" hint. Using a path
	// inside ANOTHER tmp dir guarantees it's outside both the
	// vault and the test user's home in CI environments.
	_, h, _ := reposcanTestServer(t)
	outside := t.TempDir() // distinct tmp, not under the server's vault root
	// We also can't write under HOME in CI safely. Override HOME
	// to point at the vault so "outside" is unambiguously rejected.
	t.Setenv("HOME", "/tmp/granit-test-fake-home-"+t.Name())
	code, body := scanPost(t, h, map[string]string{"path": outside})
	if code != http.StatusForbidden {
		t.Errorf("outside roots: status %d, want 403; body=%s", code, body)
	}
}

func TestHandleScanRepo_404OnMissingPath(t *testing.T) {
	_, h, root := reposcanTestServer(t)
	missing := filepath.Join(root, "does-not-exist")
	code, _ := scanPost(t, h, map[string]string{"path": missing})
	if code != http.StatusNotFound {
		t.Errorf("missing path: status %d, want 404", code)
	}
}

func TestHandleScanRepo_400OnPathTraversal(t *testing.T) {
	_, h, root := reposcanTestServer(t)
	bad := root + "/../../etc"
	code, _ := scanPost(t, h, map[string]string{"path": bad})
	if code != http.StatusBadRequest {
		t.Errorf("path traversal: status %d, want 400", code)
	}
}
