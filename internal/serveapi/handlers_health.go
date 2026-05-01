package serveapi

import "net/http"

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"root":  s.cfg.Vault.Root,
		"notes": s.cfg.Vault.NoteCount(),
	})
}
