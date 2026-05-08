package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/aiprefs"
	"github.com/artaeon/granit/internal/goals"
	"github.com/artaeon/granit/internal/granitmeta"
	"github.com/artaeon/granit/internal/ventures"
)

// AI link suggester. Given the current note's path + buffer, returns
// candidate tags + outbound links (notes / projects / goals / ventures)
// drawn from a curated candidate pool. The model is told to pick ONLY
// from the supplied candidates for entity links — preventing phantom
// refs like "[[some made-up note]]" — but is free to invent tags since
// tags are by nature open-ended.
//
// Why this exists: wiki-links are a manual chore. The user remembers
// they wrote about a topic before, but not the exact title. The vault
// graph stays sparse because of friction. Surfacing AI-proposed links
// at save time fixes the friction without forcing semi-curated graph
// maintenance — accept/reject chips, no commit until the user agrees.

const suggestLinksSystemPrompt = `You will receive a note (path + content) and a candidate pool of vault entities (notes, projects, goals, ventures) plus existing tags.

Return STRICTLY a JSON object (no prose, no markdown fences) with this shape:
{
  "tags": [{"name": "lowercase-hyphenated-tag", "rationale": "<8 words"}],
  "links": [{"type": "note"|"project"|"goal"|"venture", "ref": "<exact ref from candidates>", "title": "<the candidate's display title>", "rationale": "<10 words"}]
}

Rules:
- "links" entries MUST come from the supplied candidate pool. NEVER invent refs.
- Match candidates whose subject is genuinely related to the note's content. Quality over quantity. 2-4 links is plenty; 0 is fine if nothing fits.
- "tags" should be 3-5 short tags relevant to the note. Prefer reusing existing tags from the pool when they fit. Coin a new tag only if no existing one matches.
- "ref" for notes is the path. For projects/goals/ventures it is the name/title.
- Skip the note's own path. Skip tags already on this note.
- Lowercase, hyphenated tags. No emoji.`

type linkCandidate struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	Status  string `json:"status,omitempty"`
}

type suggestLinksRequest struct {
	NotePath    string   `json:"note_path"`
	Content     string   `json:"content"`
	ExistingTags []string `json:"existing_tags,omitempty"`
}

type suggestedTag struct {
	Name      string `json:"name"`
	Rationale string `json:"rationale,omitempty"`
}

type suggestedLink struct {
	Type      string `json:"type"`
	Ref       string `json:"ref"`
	Title     string `json:"title,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

type suggestLinksResponse struct {
	Tags  []suggestedTag  `json:"tags"`
	Links []suggestedLink `json:"links"`
}

func (s *Server) handleAISuggestLinks(w http.ResponseWriter, r *http.Request) {
	var req suggestLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusOK, suggestLinksResponse{Tags: []suggestedTag{}, Links: []suggestedLink{}})
		return
	}
	// Cap content at ~12k chars so the prompt stays bounded for big
	// notes. The first ~12k of a note carries enough signal for tag
	// + link inference; the model doesn't need the long tail.
	if len(content) > 12000 {
		content = content[:12000] + "\n…(truncated)"
	}
	candidates := s.buildLinkCandidates(req.NotePath)
	pool, _ := json.Marshal(map[string]interface{}{
		"candidates":    candidates,
		"existing_tags": req.ExistingTags,
	})
	userPrompt := fmt.Sprintf(
		"Note path: %s\n\n--- Note content ---\n%s\n--- end note ---\n\nCandidate pool + existing tags:\n```json\n%s\n```",
		req.NotePath, content, string(pool))
	out, err := s.runAIFeature(r.Context(), aiprefs.FeatureSuggestLinks, suggestLinksSystemPrompt, userPrompt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cleaned := stripJSONFences(out)
	var parsed suggestLinksResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		// Be honest — return empty + raw so the UI can show a "model
		// didn't return JSON" hint without crashing.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tags":    []suggestedTag{},
			"links":   []suggestedLink{},
			"raw":     out,
			"warning": "Model didn't return parseable JSON.",
		})
		return
	}
	// Defensive filter: drop any link whose ref isn't in the candidate
	// pool. Models occasionally hallucinate paths even when told not
	// to; we fail closed rather than surface phantom refs to the user.
	allowed := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		allowed[c.Type+"|"+c.Ref] = struct{}{}
	}
	filtered := parsed.Links[:0]
	for _, l := range parsed.Links {
		if _, ok := allowed[l.Type+"|"+l.Ref]; !ok {
			continue
		}
		// Skip link to self.
		if l.Type == "note" && l.Ref == req.NotePath {
			continue
		}
		filtered = append(filtered, l)
	}
	parsed.Links = filtered
	// Skip tags already on the note.
	if len(req.ExistingTags) > 0 {
		have := make(map[string]struct{}, len(req.ExistingTags))
		for _, t := range req.ExistingTags {
			have[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
		}
		kept := parsed.Tags[:0]
		for _, t := range parsed.Tags {
			if _, ok := have[strings.ToLower(strings.TrimSpace(t.Name))]; ok {
				continue
			}
			kept = append(kept, t)
		}
		parsed.Tags = kept
	}
	writeJSON(w, http.StatusOK, parsed)
}

// buildLinkCandidates assembles the pool the model picks from.
// Cap each entity type so the prompt stays under control on big
// vaults. Recency-sorted notes carry the most signal — the user is
// likely to link to something they touched recently.
func (s *Server) buildLinkCandidates(currentPath string) []linkCandidate {
	var out []linkCandidate

	// Notes: top 60 by mod time, excluding the current note.
	if s.cfg.Vault != nil {
		type rec struct {
			path string
			mod  time.Time
		}
		var notes []rec
		for path, n := range s.cfg.Vault.Notes {
			if path == currentPath {
				continue
			}
			notes = append(notes, rec{path, n.ModTime})
		}
		sort.Slice(notes, func(i, j int) bool { return notes[i].mod.After(notes[j].mod) })
		if len(notes) > 60 {
			notes = notes[:60]
		}
		for _, p := range notes {
			n := s.cfg.Vault.GetNote(p.path)
			if n == nil {
				continue
			}
			ex := strings.TrimSpace(stripFrontmatterBody(n.Content))
			if len(ex) > 120 {
				ex = ex[:120] + "…"
			}
			out = append(out, linkCandidate{
				Type:    "note",
				Ref:     n.RelPath,
				Title:   n.Title,
				Excerpt: ex,
			})
		}
	}

	// Projects: all active.
	if projs, err := granitmeta.ReadProjects(s.cfg.Vault.Root); err == nil {
		for _, p := range projs {
			if p.Status != "" && p.Status != "active" {
				continue
			}
			out = append(out, linkCandidate{
				Type:    "project",
				Ref:     p.Name,
				Title:   p.Name,
				Excerpt: p.Description,
				Status:  p.Status,
			})
		}
	}

	// Goals: active only.
	for _, g := range goals.LoadAll(s.cfg.Vault.Root) {
		if g.Status != "" && g.Status != "active" {
			continue
		}
		out = append(out, linkCandidate{
			Type:   "goal",
			Ref:    g.Title,
			Title:  g.Title,
			Status: string(g.Status),
		})
	}

	// Ventures: active only.
	for _, v := range ventures.LoadAll(s.cfg.Vault.Root) {
		if v.Status != "" && v.Status != "active" {
			continue
		}
		out = append(out, linkCandidate{
			Type:   "venture",
			Ref:    v.Name,
			Title:  v.Name,
			Status: string(v.Status),
		})
	}

	return out
}

// stripJSONFences trims a leading ```json ... ``` wrapper if a model
// returned one despite our instructions. Same pattern as inbox-
// triage; lifted here so suggest-links doesn't depend on order of
// declarations in handlers_ai_features.go.
func stripJSONFences(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```json")
		t = strings.TrimPrefix(t, "```")
		t = strings.TrimSuffix(t, "```")
		t = strings.TrimSpace(t)
	}
	return t
}
