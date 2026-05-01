package serveapi

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/agents"
	"github.com/artaeon/granit/internal/vault"
)

// presetView is the JSON shape we send to the web. We omit the long
// SystemPrompt by default — the user rarely needs to read it, but we
// expose it via ?include=prompt for the rare case (e.g. "explain what
// this preset does to the LLM").
type presetView struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Tools        []string `json:"tools"`
	IncludeWrite bool     `json:"includeWrite"`
	SystemPrompt string   `json:"systemPrompt,omitempty"`
	Source       string   `json:"source"` // "builtin" | "vault"
}

// handleListAgentPresets returns built-in presets merged with the
// vault-local override directory (.granit/agents/<id>.json). The Source
// field tells the UI which icon/badge to show.
func (s *Server) handleListAgentPresets(w http.ResponseWriter, r *http.Request) {
	includePrompt := r.URL.Query().Get("include") == "prompt"

	cat := agents.NewPresetCatalog(agents.BuiltinPresets())
	overridden := map[string]bool{}
	if _, errs := cat.LoadVaultDir(s.cfg.Vault.Root); len(errs) > 0 {
		// Soft-fail: invalid override files shouldn't make the page
		// 500. We log and surface a list to the user for diagnosis.
		for _, e := range errs {
			s.cfg.Logger.Warn("agent preset override invalid", "err", e)
		}
	}
	// Walk the vault dir again to mark which IDs were sourced from disk.
	// (LoadVaultDir doesn't return that map directly.)
	overridden = scanAgentOverrideIDs(s.cfg.Vault.Root)

	all := cat.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]presetView, len(all))
	for i, p := range all {
		v := presetView{
			ID:           p.ID,
			Name:         p.Name,
			Description:  p.Description,
			Tools:        p.Tools,
			IncludeWrite: p.IncludeWrite,
			Source:       "builtin",
		}
		if overridden[p.ID] {
			v.Source = "vault"
		}
		if includePrompt {
			v.SystemPrompt = p.SystemPrompt
		}
		out[i] = v
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"presets": out, "total": len(out)})
}

// scanAgentOverrideIDs walks .granit/agents/*.json and returns the set
// of IDs that ship as vault-local overrides.
func scanAgentOverrideIDs(vaultRoot string) map[string]bool {
	out := map[string]bool{}
	cat := agents.NewPresetCatalog(nil)
	if _, errs := cat.LoadVaultDir(vaultRoot); errs != nil {
		// We still want to enumerate what loaded successfully — the
		// catalog reflects exactly the IDs that got picked up.
		_ = errs
	}
	for _, p := range cat.All() {
		out[p.ID] = true
	}
	return out
}

// runView is the row shape for the runs list. Each agent run is
// persisted as a typed-object note in the Agents/ folder; we surface
// the metadata granit writes into frontmatter.
type runView struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Preset  string `json:"preset"`
	Goal    string `json:"goal"`
	Status  string `json:"status"`
	Started string `json:"started"`
	Steps   int    `json:"steps"`
	Model   string `json:"model,omitempty"`
}

// handleListAgentRuns scans every note whose frontmatter type is
// "agent_run" and returns a sorted timeline (newest first). Cheap
// because we already have the vault index loaded.
func (s *Server) handleListAgentRuns(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	notes := s.cfg.Vault.SnapshotNotes()
	out := make([]runView, 0, 32)
	for _, n := range notes {
		// agent_run notes live in Agents/. Cheap fast-path before we
		// pay the cost of parsing frontmatter on every note in the vault.
		if !strings.HasPrefix(n.RelPath, "Agents/") {
			continue
		}
		s.cfg.Vault.EnsureLoaded(n.RelPath)
		if !isAgentRun(n) {
			continue
		}
		out = append(out, noteToRunView(n))
	}
	// Newest first by Started date (fallback: path).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Started != out[j].Started {
			return out[i].Started > out[j].Started
		}
		return out[i].Path > out[j].Path
	})
	if len(out) > limit {
		out = out[:limit]
	}

	// Per-preset summary so the UI can show "research-synthesizer: 12 runs, 11 ok".
	stats := map[string]map[string]int{}
	for _, n := range notes {
		if !strings.HasPrefix(n.RelPath, "Agents/") {
			continue
		}
		s.cfg.Vault.EnsureLoaded(n.RelPath)
		if !isAgentRun(n) {
			continue
		}
		preset := frontStr(n, "preset")
		status := frontStr(n, "status")
		if preset == "" {
			preset = "(unknown)"
		}
		if status == "" {
			status = "ok"
		}
		if stats[preset] == nil {
			stats[preset] = map[string]int{}
		}
		stats[preset][status]++
		stats[preset]["total"]++
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runs":  out,
		"total": len(out),
		"stats": stats,
	})
}

func isAgentRun(n *vault.Note) bool {
	return frontStr(n, "type") == "agent_run"
}

func noteToRunView(n *vault.Note) runView {
	return runView{
		Path:    n.RelPath,
		Title:   firstNonEmpty(frontStr(n, "title"), n.Title),
		Preset:  frontStr(n, "preset"),
		Goal:    frontStr(n, "goal"),
		Status:  firstNonEmpty(frontStr(n, "status"), "ok"),
		Started: frontStr(n, "started"),
		Steps:   frontInt(n, "steps"),
		Model:   frontStr(n, "model"),
	}
}

// frontStr returns a frontmatter string value with surrounding quotes
// stripped. The vault's parser is intentionally naive (stores raw strings),
// so a value written as `title: "Foo"` in YAML comes back as `"Foo"` —
// we unquote here to keep the API clean.
func frontStr(n *vault.Note, key string) string {
	if n.Frontmatter == nil {
		return ""
	}
	v, ok := n.Frontmatter[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return unquoteYAMLString(s)
}

// frontInt extracts a number from frontmatter. The naive parser stores
// numbers as strings ("7"), but a more rigorous YAML parser would emit
// int/float64 — we accept both so the function survives a parser swap.
func frontInt(n *vault.Note, key string) int {
	if n.Frontmatter == nil {
		return 0
	}
	v, ok := n.Frontmatter[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(unquoteYAMLString(t))); err == nil {
			return n
		}
	}
	return 0
}

func unquoteYAMLString(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

