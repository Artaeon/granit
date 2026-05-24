package serveapi

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/vision"
	"github.com/artaeon/granit/internal/visions"
	"github.com/artaeon/granit/internal/wshub"
)

// HTTP layer for the multi-document vision catalogue.
//
// Routes (registered in server.go):
//   GET    /api/v1/visions             → full Store + lazy legacy migration
//   GET    /api/v1/visions/{key}       → single Doc
//   PUT    /api/v1/visions/{key}       → update Content + append History (body: {content, reason})
//   POST   /api/v1/visions             → create new custom Doc        (body: {key, label, content, reason?})
//   POST   /api/v1/visions/{key}/pin   → mark this doc as the today-view surface (unpins others)
//
// Distinct from /api/v1/vision (singular), which still owns the
// legacy values + season_focus + notes sidecar.

const statePathVisions = ".granit/visions.json"

func (s *Server) bcastVisions() {
	s.hub.Broadcast(wshub.Event{Type: "state.changed", Path: statePathVisions})
}

// ensureVisionsMigrated loads the catalogue, seeding it with the
// six default docs on first call and folding the legacy mission
// text from .granit/vision.json into the new 'mission' doc. The
// file-exists check is what makes this idempotent: once the
// catalogue is on disk, this just Loads without touching the
// legacy mission again.
func (s *Server) ensureVisionsMigrated() (visions.Store, error) {
	if _, err := os.Stat(visions.StatePath(s.cfg.Vault.Root)); err == nil {
		return visions.Load(s.cfg.Vault.Root)
	}
	store := visions.SeedStore()
	legacy := vision.Load(s.cfg.Vault.Root)
	if legacy.Mission != "" {
		if d := visions.FindDoc(&store, "mission"); d != nil {
			d.Content = legacy.Mission
			d.UpdatedAt = time.Now().UTC()
		}
	}
	if err := visions.Save(s.cfg.Vault.Root, store); err != nil {
		return visions.Store{}, err
	}
	return store, nil
}

func (s *Server) handleListVisions(w http.ResponseWriter, r *http.Request) {
	store, err := s.ensureVisionsMigrated()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, store)
}

func (s *Server) handleGetVisionDoc(w http.ResponseWriter, r *http.Request) {
	key := urlParam(r, "key")
	store, err := s.ensureVisionsMigrated()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d := visions.FindDoc(&store, key)
	if d == nil {
		writeError(w, http.StatusNotFound, "vision not found")
		return
	}
	writeJSON(w, http.StatusOK, *d)
}

type putVisionBody struct {
	Content string `json:"content"`
	Reason  string `json:"reason"`
}

// handlePutVisionDoc updates the named doc's Content and pushes a
// history entry with the user's reason. The reason is required —
// the whole point of this feature is making vision edits intentional
// and reviewable, so saving without typing why doesn't fit the
// model. (Initial migration from legacy mission is exempt because
// it's not a user edit; it runs in ensureVisionsMigrated above.)
func (s *Server) handlePutVisionDoc(w http.ResponseWriter, r *http.Request) {
	key := urlParam(r, "key")
	var body putVisionBody
	if !readJSON(w, r, &body) {
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" {
		writeError(w, http.StatusBadRequest, "reason required — say why you're changing this")
		return
	}
	store, err := s.ensureVisionsMigrated()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d := visions.FindDoc(&store, key)
	if d == nil {
		writeError(w, http.StatusNotFound, "vision not found")
		return
	}
	visions.ApplyEdit(d, body.Content, body.Reason)
	if err := visions.Save(s.cfg.Vault.Root, store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVisions()
	writeJSON(w, http.StatusOK, *d)
}

type createVisionBody struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
}

// handleCreateVisionDoc adds a new custom doc beyond the six seeded
// keys. Body must include Key (kebab-case, used in the URL) and
// Label (display name). Content + reason are optional on create —
// a user might want to seed an empty "Family" tab and write into it
// later.
func (s *Server) handleCreateVisionDoc(w http.ResponseWriter, r *http.Request) {
	var body createVisionBody
	if !readJSON(w, r, &body) {
		return
	}
	body.Key = strings.TrimSpace(body.Key)
	body.Label = strings.TrimSpace(body.Label)
	if body.Key == "" || body.Label == "" {
		writeError(w, http.StatusBadRequest, "key and label required")
		return
	}
	// Lightweight key validation — kebab-case ASCII so the key works
	// as a URL segment without escaping. Reject obvious bad shapes
	// (spaces / control chars / slashes); we don't enforce a strict
	// regex because rejecting "über" or "körper" would be hostile.
	for _, ch := range body.Key {
		if ch == '/' || ch == ' ' || ch < 0x20 {
			writeError(w, http.StatusBadRequest, "key may not contain spaces, slashes, or control characters")
			return
		}
	}
	store, err := s.ensureVisionsMigrated()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if visions.FindDoc(&store, body.Key) != nil {
		writeError(w, http.StatusConflict, "a vision with this key already exists")
		return
	}
	now := time.Now().UTC()
	doc := visions.Doc{
		Key:       body.Key,
		Label:     body.Label,
		Content:   strings.TrimSpace(body.Content),
		UpdatedAt: now,
	}
	// If the create call carries seed content AND a reason, record
	// the creation as a history entry so the user can see "I added
	// this on date X because Y" later. Empty seed content + no reason
	// means a quiet structural add; no point in a noop history row.
	if doc.Content != "" && strings.TrimSpace(body.Reason) != "" {
		doc.History = []visions.HistoryEntry{{
			When:    now,
			Reason:  strings.TrimSpace(body.Reason),
			Content: "",
		}}
	}
	store.Docs = append(store.Docs, doc)
	if err := visions.Save(s.cfg.Vault.Root, store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVisions()
	writeJSON(w, http.StatusOK, doc)
}

// handlePinVisionDoc marks the named doc as the today-view surface
// and unpins everything else (single-slot Kurzvision). No body
// required.
func (s *Server) handlePinVisionDoc(w http.ResponseWriter, r *http.Request) {
	key := urlParam(r, "key")
	store, err := s.ensureVisionsMigrated()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if visions.FindDoc(&store, key) == nil {
		writeError(w, http.StatusNotFound, "vision not found")
		return
	}
	for i := range store.Docs {
		store.Docs[i].Pinned = store.Docs[i].Key == key
	}
	if err := visions.Save(s.cfg.Vault.Root, store); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastVisions()
	writeJSON(w, http.StatusOK, store)
}

