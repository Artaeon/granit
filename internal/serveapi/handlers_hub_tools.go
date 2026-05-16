// Package serveapi — handlers for /api/v1/hub/tools.
//
// The hub's tools half is a CRUD over .granit/hub-tools.json. A
// tool card carries an icon, name, optional description, and an
// ordered list of copy-paste setup commands the user pastes into
// a terminal ("brew install neovim", "kubectl config use-context
// prod", etc). Mirrors the hub-link handler shape; lives in its
// own file because the storage is separate and the CRUD has its
// own quirks (sanitise commands on every write, broadcast a
// dedicated WS event so a tab on the tools section can refresh
// without re-fetching the links).
package serveapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artaeon/granit/internal/hub"
	"github.com/artaeon/granit/internal/wshub"
)

const hubToolsStatePath = ".granit/hub-tools.json"

// bcastHubTools fires the dedicated tools-changed event so a tab
// scrolled to the tools section can refresh without also reloading
// the (potentially long) link list. Mirrors bcastHub.
func (s *Server) bcastHubTools() {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(wshub.Event{Type: "hub.tools.changed", Path: hubToolsStatePath})
}

func (s *Server) handleListHubTools(w http.ResponseWriter, r *http.Request) {
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tools == nil {
		tools = []hub.Tool{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools, "total": len(tools)})
}

func (s *Server) handleCreateHubTool(w http.ResponseWriter, r *http.Request) {
	var t hub.Tool
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	t.Description = strings.TrimSpace(t.Description)
	t.Icon = strings.TrimSpace(t.Icon)
	t.Color = strings.TrimSpace(t.Color)
	t.Commands = hub.SanitizeCommands(t.Commands)
	t.Tags = hub.SanitizeTags(t.Tags)
	if t.ID == "" {
		t.ID = hub.NewID()
	}
	now := hub.Now()
	t.CreatedAt = now
	t.UpdatedAt = now

	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tools = append(tools, t)
	if err := hub.SaveAllTools(s.cfg.Vault.Root, tools); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	writeJSON(w, http.StatusCreated, t)
}

// handlePatchHubTool — partial update. Unmarshals into a sparse
// map so the client can rename a tool, replace its commands, or
// edit a single field without sending the whole record. Mirrors
// the link patch shape.
func (s *Server) handlePatchHubTool(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	var patch map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	idx := -1
	for i, t := range tools {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	t := tools[idx]
	apply := func(key string, into interface{}) {
		if raw, ok := patch[key]; ok {
			_ = json.Unmarshal(raw, into)
		}
	}
	apply("name", &t.Name)
	apply("description", &t.Description)
	apply("icon", &t.Icon)
	apply("color", &t.Color)
	apply("tags", &t.Tags)
	apply("commands", &t.Commands)
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	t.Description = strings.TrimSpace(t.Description)
	t.Icon = strings.TrimSpace(t.Icon)
	t.Color = strings.TrimSpace(t.Color)
	t.Commands = hub.SanitizeCommands(t.Commands)
	t.Tags = hub.SanitizeTags(t.Tags)
	t.UpdatedAt = hub.Now()
	tools[idx] = t
	if err := hub.SaveAllTools(s.cfg.Vault.Root, tools); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteHubTool(w http.ResponseWriter, r *http.Request) {
	id := urlParam(r, "id")
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := tools[:0]
	found := false
	for _, t := range tools {
		if t.ID == id {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		writeError(w, http.StatusNotFound, "tool not found")
		return
	}
	if err := hub.SaveAllTools(s.cfg.Vault.Root, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	w.WriteHeader(http.StatusNoContent)
}

// starterToolSet is the curated default catalogue the user can
// seed with one click. Deliberately small (git / node / docker /
// shell) — broad-strokes coverage that almost everyone uses,
// editable once seeded. The "load starter set" button calls into
// this; we never inject these silently on first visit because the
// user-owned data model means the tools are theirs to delete and
// silent injection would feel presumptuous.
func starterToolSet() []hub.Tool {
	return []hub.Tool{
		{
			Name:        "git",
			Description: "Version control basics",
			Icon:        "🐙",
			Color:       "orange",
			Tags:        []string{"vcs", "shell"},
			Commands: []hub.Command{
				{Label: "install via brew (macOS)", Command: "brew install git"},
				{Label: "configure identity", Command: "git config --global user.name \"Your Name\"\ngit config --global user.email you@example.com"},
				{Label: "clone a repo", Command: "git clone git@github.com:org/repo.git"},
				{Label: "show working tree status", Command: "git status"},
				{Label: "create + push a feature branch", Command: "git checkout -b feature/x && git push -u origin HEAD"},
			},
		},
		{
			Name:        "Node + pnpm",
			Description: "JavaScript runtime + package manager",
			Icon:        "📦",
			Color:       "green",
			Tags:        []string{"javascript", "package-manager"},
			Commands: []hub.Command{
				{Label: "install fnm (node version manager)", Command: "brew install fnm"},
				{Label: "install latest LTS Node", Command: "fnm install --lts && fnm use lts-latest"},
				{Label: "enable pnpm via corepack", Command: "corepack enable && corepack prepare pnpm@latest --activate"},
				{Label: "install project deps", Command: "pnpm install"},
				{Label: "run dev script", Command: "pnpm dev"},
			},
		},
		{
			Name:        "Docker",
			Description: "Container runtime — daily commands",
			Icon:        "🐳",
			Color:       "blue",
			Tags:        []string{"containers", "devops"},
			Commands: []hub.Command{
				{Label: "list running containers", Command: "docker ps"},
				{Label: "tail a container's logs", Command: "docker logs -f <container>"},
				{Label: "shell into a container", Command: "docker exec -it <container> /bin/sh"},
				{Label: "prune everything unused (careful!)", Command: "docker system prune -a --volumes", Notes: "Removes stopped containers + dangling images + unused volumes. Read the prompt."},
				{Label: "build + tag from a Dockerfile", Command: "docker build -t myapp:dev ."},
			},
		},
		{
			Name:        "Shell snippets",
			Description: "Frequently-reached-for one-liners",
			Icon:        "💡",
			Color:       "purple",
			Tags:        []string{"shell", "snippets"},
			Commands: []hub.Command{
				{Label: "find files by name (case-insensitive)", Command: "find . -iname '*pattern*'"},
				{Label: "grep recursively for a string", Command: "grep -rn 'needle' ."},
				{Label: "what's listening on port 3000", Command: "lsof -i :3000"},
				{Label: "kill process on a port", Command: "lsof -ti :3000 | xargs kill -9"},
				{Label: "human-readable disk usage", Command: "du -sh */ | sort -h"},
			},
		},
	}
}

// handleSeedHubTools appends the starter set to the existing
// catalogue. Deliberately additive (not destructive): if the user
// already curated a tools list, seeding adds the defaults rather
// than overwriting their work. Each starter card lands as a fresh
// record with its own ULID + timestamps so the user can edit /
// delete freely afterwards — same user-owned data lifecycle as a
// hand-rolled entry.
func (s *Server) handleSeedHubTools(w http.ResponseWriter, r *http.Request) {
	tools, err := hub.LoadAllTools(s.cfg.Vault.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Skip starters whose name already exists (case-insensitive)
	// so a double-click doesn't end up with two "git" cards.
	existing := map[string]bool{}
	for _, t := range tools {
		existing[strings.ToLower(t.Name)] = true
	}
	now := hub.Now()
	added := 0
	for _, t := range starterToolSet() {
		if existing[strings.ToLower(t.Name)] {
			continue
		}
		t.ID = hub.NewID()
		t.CreatedAt = now
		t.UpdatedAt = now
		t.Commands = hub.SanitizeCommands(t.Commands)
		t.Tags = hub.SanitizeTags(t.Tags)
		tools = append(tools, t)
		added++
	}
	if added == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"added": 0, "total": len(tools)})
		return
	}
	if err := hub.SaveAllTools(s.cfg.Vault.Root, tools); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "total": len(tools)})
}

// handleReorderHubTools rewrites SortOrder on the given IDs in
// the supplied order. Tools not in the list keep their existing
// SortOrder so a partial reorder (drag one card to the top of a
// long list) doesn't trash the rest of the catalogue.
func (s *Server) handleReorderHubTools(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids required")
		return
	}
	if err := hub.ReorderTools(s.cfg.Vault.Root, body.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.bcastHubTools()
	w.WriteHeader(http.StatusNoContent)
}
