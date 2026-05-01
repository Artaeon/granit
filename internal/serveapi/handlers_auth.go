package serveapi

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleAuthStatus is the only auth endpoint reachable BEFORE the user
// has any token — the web app calls it on first paint to decide:
//   - hasPassword=false → render the "set up your password" form
//   - hasPassword=true  → render the login form
//
// It deliberately leaks no other information. SetupAt is included so
// the user sees "Account created on …" for confidence.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"hasPassword": false})
		return
	}
	out := map[string]any{
		"hasPassword":  s.auth.HasPassword(),
		"sessionCount": s.auth.SessionCount(),
	}
	if s.auth.HasPassword() {
		s.auth.mu.RLock()
		setupAt := s.auth.store.SetupAt
		s.auth.mu.RUnlock()
		if !setupAt.IsZero() {
			out["setupAt"] = setupAt.Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAuthSetup is callable only when no password has been set yet.
// Once set, it 409s — the user must use the change-password endpoint
// (which requires the current password) to change it.
func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	if s.auth.HasPassword() {
		writeError(w, http.StatusConflict, "password already set; use change-password")
		return
	}
	var body struct {
		Password string `json:"password"`
		Label    string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.auth.SetPassword(body.Password, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tok, err := s.auth.CreateSession(body.Label)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   tok,
		"message": "password set; you are logged in",
	})
}

// handleAuthLogin trades a correct password for a fresh session token.
// On the wrong password we deliberately delay ~250ms — slow enough to
// noticeably penalize bulk attempts, fast enough to feel instant on the
// happy path. Sub-second timing channel is acceptable for a single-user
// self-hosted tool; no need for full timing safety.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeError(w, http.StatusInternalServerError, "auth not initialized")
		return
	}
	if !s.auth.HasPassword() {
		writeError(w, http.StatusForbidden, "no password set; complete setup first")
		return
	}
	var body struct {
		Password string `json:"password"`
		Label    string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !s.auth.VerifyPassword(body.Password) {
		time.Sleep(250 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	tok, err := s.auth.CreateSession(body.Label)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
	})
}

// handleAuthLogout revokes the current session (the bearer token used
// to make the request). 204 either way — logging out a non-session
// caller is a no-op.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		tok := bearerFromHeader(r)
		if tok != "" {
			s.auth.RevokeToken(tok)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthChangePassword rotates the password. Requires the OLD
// password (even though the request is already authed) — defense in
// depth against XSS or session hijack scenarios. Wipes ALL sessions,
// including the caller's, so the user must log in again everywhere.
func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || !s.auth.HasPassword() {
		writeError(w, http.StatusForbidden, "no password set")
		return
	}
	var body struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !s.auth.VerifyPassword(body.Old) {
		time.Sleep(250 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "old password incorrect")
		return
	}
	if err := s.auth.SetPassword(body.New, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthRevokeAll is the "log out everywhere" button — useful when
// the user suspects a device has been compromised. Authed (so the
// caller can prove they hold the current password's session); leaves
// the caller's session also revoked, so the UI re-routes to login.
func (s *Server) handleAuthRevokeAll(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.auth.RevokeAllSessions()
	w.WriteHeader(http.StatusNoContent)
}
