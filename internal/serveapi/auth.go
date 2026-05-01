package serveapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// requireToken accepts EITHER:
//   - the legacy bootstrap bearer token printed at server startup (used
//     by CLI scripts and the Tauri desktop wrapper), OR
//   - any valid password-login session token from authState.
//
// We keep both paths so existing automation doesn't break when the user
// later sets a password — only the displayed UX changes.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := bearerFromHeader(r)
		if got == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		if s.auth != nil && s.auth.IsValidToken(got) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusUnauthorized, "invalid token")
	})
}

func bearerFromHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, prefix))
}
