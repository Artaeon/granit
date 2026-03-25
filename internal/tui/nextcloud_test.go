package tui

import (
	"strings"
	"testing"

	"github.com/artaeon/granit/internal/config"
)

// ---------------------------------------------------------------------------
// safePath
// ---------------------------------------------------------------------------

func TestSafePath_Valid(t *testing.T) {
	abs, err := safePath("/vault", "notes/daily.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(abs, "/vault/") {
		t.Errorf("expected path under /vault/, got %q", abs)
	}
	if !strings.HasSuffix(abs, "notes/daily.md") {
		t.Errorf("expected path ending with notes/daily.md, got %q", abs)
	}
}

func TestSafePath_ParentTraversal(t *testing.T) {
	_, err := safePath("/vault", "../etc/passwd")
	if err == nil {
		t.Error("expected error for parent traversal, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected 'path traversal' error, got: %v", err)
	}
}

func TestSafePath_AbsolutePath(t *testing.T) {
	// filepath.Join("/vault", "/etc/passwd") on unix yields "/vault/etc/passwd"
	// which is actually safe. But "../../etc/passwd" should fail:
	_, err := safePath("/vault", "../../etc/passwd")
	if err == nil {
		t.Error("expected error for absolute-like traversal, got nil")
	}
}

func TestSafePath_EmptyPath(t *testing.T) {
	abs, err := safePath("/vault", "")
	if err != nil {
		// Empty path resolves to vault root itself, which is allowed
		// by the condition `abs == filepath.Clean(vaultRoot)`.
		// So no error is expected.
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if abs != "/vault" {
		t.Errorf("expected '/vault', got %q", abs)
	}
}

func TestSafePath_DotFiles(t *testing.T) {
	abs, err := safePath("/vault", ".granit/config.json")
	if err != nil {
		t.Fatalf("unexpected error for dotfile: %v", err)
	}
	if !strings.Contains(abs, ".granit") {
		t.Errorf("expected path to contain .granit, got %q", abs)
	}
}

// ---------------------------------------------------------------------------
// davURL
// ---------------------------------------------------------------------------

func TestDavURL(t *testing.T) {
	nc := &NextcloudSync{
		baseURL:    "https://cloud.example.com",
		user:       "alice",
		remotePath: "/Notes",
	}

	tests := []struct {
		name    string
		relPath string
		want    string
	}{
		{
			name:    "empty path",
			relPath: "",
			want:    "https://cloud.example.com/remote.php/dav/files/alice/Notes",
		},
		{
			name:    "root slash",
			relPath: "/",
			want:    "https://cloud.example.com/remote.php/dav/files/alice/Notes",
		},
		{
			name:    "simple file",
			relPath: "daily.md",
			want:    "https://cloud.example.com/remote.php/dav/files/alice/Notes/daily.md",
		},
		{
			name:    "nested path",
			relPath: "projects/alpha/notes.md",
			want:    "https://cloud.example.com/remote.php/dav/files/alice/Notes/projects/alpha/notes.md",
		},
		{
			name:    "path with spaces",
			relPath: "My Notes/hello world.md",
			want:    "https://cloud.example.com/remote.php/dav/files/alice/Notes/My%20Notes/hello%20world.md",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nc.davURL(tc.relPath)
			if got != tc.want {
				t.Errorf("davURL(%q) = %q, want %q", tc.relPath, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// httpStatusMessage
// ---------------------------------------------------------------------------

func TestHTTPStatusMessage(t *testing.T) {
	tests := []struct {
		code     int
		contains string
	}{
		{401, "authentication"},
		{403, "forbidden"},
		{404, "not found"},
		{409, "conflict"},
		{500, "500"},
	}

	for _, tc := range tests {
		msg := httpStatusMessage(tc.code)
		if !strings.Contains(strings.ToLower(msg), strings.ToLower(tc.contains)) {
			t.Errorf("httpStatusMessage(%d) = %q, expected to contain %q", tc.code, msg, tc.contains)
		}
	}
}

// ---------------------------------------------------------------------------
// NextcloudOverlay — creation and rendering
// ---------------------------------------------------------------------------

func TestNextcloudOverlay_NewAndView(t *testing.T) {
	overlay := NewNextcloudOverlay()
	if overlay.IsActive() {
		t.Error("expected new overlay to be inactive")
	}

	cfg := config.DefaultConfig()
	cfg.NextcloudURL = "https://cloud.example.com"
	cfg.NextcloudUser = "alice"
	cfg.NextcloudPath = "/Notes"

	overlay.Open(cfg, "/tmp/vault")
	if !overlay.IsActive() {
		t.Error("expected overlay to be active after Open")
	}

	overlay.SetSize(120, 40)

	// View should not panic and should produce non-empty output
	output := overlay.View()
	if output == "" {
		t.Error("expected non-empty View() output")
	}
	if !strings.Contains(output, "Nextcloud") {
		t.Error("expected View() to contain 'Nextcloud'")
	}

	overlay.Close()
	if overlay.IsActive() {
		t.Error("expected overlay to be inactive after Close")
	}
}
