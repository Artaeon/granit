package serveapi

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// deviceView is the public shape of an authState.sessionRecord. The
// raw token never leaves the server; we expose the first 12 chars of
// its sha256 hash as a stable, opaque ID the UI can pass back to the
// revoke endpoint.
type deviceView struct {
	ID        string `json:"id"`
	Label     string `json:"label,omitempty"`
	CreatedAt string `json:"createdAt"`
	LastUsed  string `json:"lastUsed"`
	Current   bool   `json:"current"`
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"devices": []deviceView{}, "total": 0})
		return
	}

	// "current" highlight — match the bearer the caller used against
	// each session's stored hash so the UI can render a "this device" pill.
	bearer := bearerFromHeader(r)
	currentHash := ""
	if bearer != "" {
		currentHash = sha256Hex(bearer)
	}

	s.auth.mu.RLock()
	out := make([]deviceView, 0, len(s.auth.store.Sessions))
	for _, sess := range s.auth.store.Sessions {
		out = append(out, deviceView{
			ID:        deviceIDFromHash(sess.TokenHash),
			Label:     sess.Label,
			CreatedAt: sess.CreatedAt.Format(time.RFC3339),
			LastUsed:  sess.LastUsed.Format(time.RFC3339),
			Current:   sess.TokenHash == currentHash,
		})
	}
	s.auth.mu.RUnlock()

	// Newest activity first — the user will most often want to see
	// the device they were just on.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Current != out[j].Current {
			return out[i].Current // current device floats to the top
		}
		return out[i].LastUsed > out[j].LastUsed
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": out, "total": len(out)})
}

// handleRevokeDevice deletes a session by its public ID (hash prefix).
// Returns 404 if the prefix matches nothing — protects against the UI
// sending a stale ID after another revoke removed it.
func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := strings.ToLower(chi.URLParam(r, "id"))

	s.auth.mu.Lock()
	matched := false
	out := s.auth.store.Sessions[:0]
	for _, sess := range s.auth.store.Sessions {
		if deviceIDFromHash(sess.TokenHash) == id {
			matched = true
			continue
		}
		out = append(out, sess)
	}
	s.auth.store.Sessions = out
	// Best-effort: drop any matching token from the in-memory cache.
	for tok := range s.auth.tokenSet {
		if deviceIDFromHash(sha256Hex(tok)) == id {
			delete(s.auth.tokenSet, tok)
		}
	}
	s.auth.mu.Unlock()

	if !matched {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err := s.auth.save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deviceIDFromHash truncates the sha256 hash to 12 chars — long enough
// to be unique across any plausible number of sessions, short enough to
// fit in a URL or a UI badge.
func deviceIDFromHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
