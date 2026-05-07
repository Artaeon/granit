package serveapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/artaeon/granit/internal/atomicio"
)

// handleGetFile serves a non-markdown file from the vault by path. Used
// by the markdown preview to embed images via `![[image.png]]`. Markdown
// files have their own endpoint at /api/v1/notes/{path}; this one is
// for binaries (images, PDFs, audio) the preview wants to display
// inline.
//
// Path safety: the wildcard captures anything after /files/. We refuse
// absolute paths, any `..` component (even ones that would cancel
// out), and require the cleaned absolute path to be inside the vault
// root. Same paranoid containment used by tasks.resolveInVault — every
// vault file-system access in the server goes through this shape.
func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	rel := chi.URLParam(r, "*")
	if rel == "" {
		writeError(w, http.StatusBadRequest, "path required")
		return
	}
	if filepath.IsAbs(rel) {
		writeError(w, http.StatusBadRequest, "absolute path rejected")
		return
	}
	for _, part := range strings.FieldsFunc(rel, func(c rune) bool { return c == '/' || c == '\\' }) {
		if part == ".." {
			writeError(w, http.StatusBadRequest, "path traversal rejected")
			return
		}
	}
	root := s.cfg.Vault.Root
	abs := filepath.Clean(filepath.Join(root, rel))
	rootClean := filepath.Clean(root)
	if abs != rootClean && !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "path escapes vault")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Block markdown — those have their own JSON endpoint with
	// frontmatter parsing. Serving them as raw octet-streams here
	// would be confusing.
	if strings.HasSuffix(strings.ToLower(rel), ".md") {
		writeError(w, http.StatusBadRequest, "use /api/v1/notes/ for markdown")
		return
	}
	// Cache hint for static assets — the SW will already cache
	// /api/v1/* GETs, but a strong validator helps when SW is bypassed.
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, abs)
}

// Upload size cap. 25 MiB is generous for any image users would
// paste into a note (including high-res screenshots, simple PDFs);
// larger files are usually a sign of accidental upload and should be
// surfaced rather than silently accepted.
const uploadMaxBytes = 25 << 20

// Allowed MIME prefixes for paste-and-drop. Restricted to media +
// pdf rather than wildcard so a user dragging a binary executable
// into the editor doesn't accidentally land it in their vault.
var uploadAllowedPrefixes = []string{
	"image/",
	"audio/",
	"video/",
	"application/pdf",
}

// handleUpload accepts a multipart-form upload and writes the file
// under <vault>/attachments/YYYY/MM/<uuid>-<safe-name>. Returns the
// vault-relative path so the frontend can splice
// `![[attachments/...]]` into the editor at the cursor.
//
// Naming: we prepend an 8-char random hex to avoid collisions when
// the user pastes two screenshots with identical browser-suggested
// names ("Screenshot.png", "image.png"). The original name (sanitised)
// is preserved as a suffix so the file remains human-recognisable in
// a file browser.
//
// Path safety: same containment as handleGetFile — we construct the
// destination path ourselves and verify it stays under vaultRoot
// before writing. The user has no input on directory names; only
// the original filename is used (and sanitised).
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(uploadMaxBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	if header.Size > uploadMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds %d bytes", uploadMaxBytes))
		return
	}
	contentType := header.Header.Get("Content-Type")
	allowed := false
	for _, p := range uploadAllowedPrefixes {
		if strings.HasPrefix(contentType, p) {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "content type "+contentType+" not allowed")
		return
	}

	// Pick destination path: attachments/YYYY/MM/<rand>-<safename>.
	now := time.Now()
	year := now.Format("2006")
	month := now.Format("01")
	safe := safeFilename(header.Filename)
	if safe == "" {
		safe = "upload"
	}
	var randBytes [4]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "rng failure")
		return
	}
	rel := filepath.ToSlash(filepath.Join("attachments", year, month, hex.EncodeToString(randBytes[:])+"-"+safe))
	abs := filepath.Join(s.cfg.Vault.Root, filepath.FromSlash(rel))

	// Containment check (defense in depth — safeFilename should already
	// have stripped traversal but verify the final path lands inside
	// the vault).
	rootClean := filepath.Clean(s.cfg.Vault.Root)
	if !strings.HasPrefix(filepath.Clean(abs), rootClean+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, "destination escapes vault")
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Read the body and stage via atomicio so a crash mid-upload
	// doesn't leave a half-written file in the attachments tree.
	body, err := io.ReadAll(io.LimitReader(file, uploadMaxBytes+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read body: "+err.Error())
		return
	}
	if int64(len(body)) > uploadMaxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds %d bytes", uploadMaxBytes))
		return
	}
	if err := atomicio.WriteNote(abs, string(body)); err != nil {
		writeError(w, http.StatusInternalServerError, "write: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"path":        rel,
		"contentType": contentType,
	})
}

// safeFilename returns a filesystem-safe version of name: keeps
// letters, digits, dot, dash, underscore; everything else becomes
// '-'. Strips leading dots so the output never starts with a dotted
// component (which would create a hidden file). Leading/trailing
// dashes are trimmed; consecutive dashes collapse.
func safeFilename(name string) string {
	// Drop any path component the browser may have included.
	name = filepath.Base(name)
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-.")
	return out
}
