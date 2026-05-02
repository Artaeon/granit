package serveapi

import (
	"crypto/subtle"
	"net/http"

	"github.com/coder/websocket"
)

// handleWS accepts an authenticated WebSocket connection and forwards
// hub events for the duration of the session. Authentication mirrors
// requireToken (bootstrap bearer OR password-login session token).
//
// Token sources, in order:
//   - Authorization: Bearer <tok> header (preferred)
//   - sec-websocket-protocol subprotocol (browsers can set this on
//     `new WebSocket(url, [subprotocol])` even though they can't set
//     custom headers — the server echoes it back to negotiate)
//   - ?token=<tok> URL param (legacy; used by older web builds — kept
//     so a deploy-mid-session doesn't 401, but new code uses headers
//     or subprotocol)
//
// We deliberately do NOT log the URL on connect so the legacy
// query-param path doesn't leave the token in access logs.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	tok := bearerFromHeader(r)
	if tok == "" {
		// sec-websocket-protocol arrives as a comma-separated list.
		// We use the form `granit.<token>` so a future second
		// subprotocol doesn't collide with the token namespace.
		if proto := r.Header.Get("sec-websocket-protocol"); proto != "" {
			for _, p := range splitCSV(proto) {
				if len(p) > 7 && p[:7] == "granit." {
					tok = p[7:]
					break
				}
			}
		}
	}
	if tok == "" {
		tok = r.URL.Query().Get("token")
	}
	if !s.tokenAuthorized(tok) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// OriginPatterns + Subprotocols both must be set for browser
	// clients. We accept any origin (single-user self-host; the
	// reverse-proxy enforces TLS + CORS at the edge) but require a
	// proper Sec-WebSocket-Protocol echo when the client sent one.
	opts := &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	}
	if proto := r.Header.Get("sec-websocket-protocol"); proto != "" {
		// Echo back the granit.* subprotocol (whatever the client
		// sent). websocket.Accept refuses without a match.
		opts.Subprotocols = splitCSV(proto)
	}

	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		s.cfg.Logger.Warn("ws accept failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	s.hub.Subscribe(r.Context(), conn)
}

// tokenAuthorized returns true for the bootstrap token OR any valid
// session token. Mirrors requireToken's policy so HTTP and WS share
// the same auth surface.
func (s *Server) tokenAuthorized(tok string) bool {
	if tok == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.Token)) == 1 {
		return true
	}
	if s.auth != nil && s.auth.IsValidToken(tok) {
		return true
	}
	return false
}

// splitCSV is the trimmed comma-split we need for the
// sec-websocket-protocol header. Tiny helper; not worth importing
// strings just for this.
func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			// Trim leading/trailing spaces in the slice.
			a, b := start, i
			for a < b && (s[a] == ' ' || s[a] == '\t') {
				a++
			}
			for b > a && (s[b-1] == ' ' || s[b-1] == '\t') {
				b--
			}
			if a < b {
				out = append(out, s[a:b])
			}
			start = i + 1
		}
	}
	return out
}
