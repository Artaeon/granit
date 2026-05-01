package serveapi

import (
	"crypto/subtle"
	"net/http"

	"github.com/coder/websocket"
)

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	if tok == "" {
		tok = bearerFromHeader(r)
	}
	if tok == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // single-user self-host
	})
	if err != nil {
		s.cfg.Logger.Warn("ws accept failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	s.hub.Subscribe(r.Context(), conn)
}
