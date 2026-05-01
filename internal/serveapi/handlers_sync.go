package serveapi

import "net/http"

// SetSyncer plugs an active Syncer into the server so /api/v1/sync can
// surface its status. nil-safe: if no Syncer is set, the endpoint reports
// {enabled: false}.
func (s *Server) SetSyncer(sync *Syncer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncer = sync
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, syncer.Status())
}

func (s *Server) handleSyncTrigger(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	syncer := s.syncer
	s.mu.Unlock()
	if syncer == nil {
		writeError(w, http.StatusBadRequest, "sync is not enabled (start granit web with --sync)")
		return
	}
	go syncer.syncOnce()
	writeJSON(w, http.StatusAccepted, map[string]interface{}{"triggered": true})
}
