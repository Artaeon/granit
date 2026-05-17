package serveapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/scripture"
)

// handleScriptureSemanticSearch finds verses by meaning, not substring.
// "verses about waiting on God" is the canonical query — the existing
// substring filter on /scripture only matches literal tokens ("wait"),
// and the topical-chip strip only matches single-word themes ("hope").
// Neither helps with a sentence-shaped query.
//
// Strategy: AI passthrough through the topic index, not the verses
// themselves. Sending all ~200 bundled verses to the model would burn
// tokens for every query; the topics list is ~30 short strings and
// already does the conceptual chunking. We ask the model to pick 1-3
// topics matching the query, then the handler fetches verses for each
// topic locally. The model never returns verse text — only topic ids
// from a closed set — so hallucinated refs aren't possible.
//
// Bounded LLM call (~30s timeout); same audit + redaction gate the
// chat surface goes through. When the catalogue carries no topic
// metadata (a user-edited scriptures.md replaced the defaults) the
// handler falls back to returning the empty set with a hint —
// semantic search needs topic structure to anchor against.
type semanticSearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"` // verses returned, default 8
}

type semanticSearchResponse struct {
	Topics    []string             `json:"topics"`    // topics the model picked, in rank order
	Scriptures []scripture.Scripture `json:"scriptures"` // verses for those topics, deduped
	Query     string               `json:"query"`
}

func (s *Server) handleScriptureSemanticSearch(w http.ResponseWriter, r *http.Request) {
	var body semanticSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	query := strings.TrimSpace(body.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "query required")
		return
	}
	limit := body.Limit
	if limit <= 0 || limit > 50 {
		limit = 8
	}

	topics := scripture.Topics(s.cfg.Vault.Root)
	if len(topics) == 0 {
		// User-edited scriptures.md with no topic metadata — semantic
		// search has nothing to anchor against. Surface clearly rather
		// than silently returning everything.
		writeJSON(w, http.StatusOK, semanticSearchResponse{
			Topics:    nil,
			Scriptures: nil,
			Query:     query,
		})
		return
	}

	cfg := config.LoadForVault(s.cfg.Vault.Root)
	llm, err := agentruntime.NewLLM(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if hint := preflightLLM(llm); hint != "" {
		writeError(w, http.StatusBadRequest, hint)
		return
	}
	chatter, ok := llm.(agentruntime.Chatter)
	if !ok {
		writeError(w, http.StatusBadRequest, "configured LLM does not support chat")
		return
	}

	// Compact prompt: just the topic names. The model picks 1-3.
	// Response shape is one topic per line; we tolerate prose padding
	// (some models add "Here are…" prefixes) by parsing line-by-line
	// and matching against the known topic set.
	var topicNames []string
	for _, t := range topics {
		topicNames = append(topicNames, t.Topic)
	}
	system := "You are a scripture topical-search assistant. Given a user query and a list of available topic tags, " +
		"return the 1-3 topics that best match the query — one per line, no preamble, no commentary, no markdown. " +
		"Return ONLY topic names from the provided list, exactly as spelled. If nothing fits, return one line: NONE.\n\n" +
		"Available topics:\n" + strings.Join(topicNames, "\n")
	user := "Query: " + query

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	wire := []agentruntime.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	// Run through the same redaction + sabbath gate the chat surface
	// uses so semantic search isn't a side channel that bypasses AI
	// preferences.
	gated, redactStats, gateErr := s.gateChat(wire)
	if gateErr != nil {
		writeError(w, http.StatusBadRequest, gateErr.Error())
		return
	}
	reply, err := chatter.Chat(ctx, gated)
	s.auditChat(cfg, llm, len(reply), redactStats, err, transcriptForHash(gated))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Parse: one topic per line, case-insensitive match against
	// the known set. Tolerate leading bullet markers / numbering /
	// quotes in case the model decorates despite instructions.
	known := make(map[string]string, len(topics))
	for _, t := range topics {
		known[strings.ToLower(strings.TrimSpace(t.Topic))] = t.Topic
	}
	var picked []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "-*•1234567890. \"'")
		line = strings.Trim(line, " \"'.")
		if line == "" || strings.EqualFold(line, "NONE") {
			continue
		}
		canonical, ok := known[strings.ToLower(line)]
		if !ok {
			continue
		}
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		picked = append(picked, canonical)
		if len(picked) >= 3 {
			break
		}
	}

	// Collect verses for the picked topics, deduping by source+text
	// so a verse tagged with two picked topics doesn't appear twice.
	var verses []scripture.Scripture
	verseSeen := make(map[string]bool)
	for _, topic := range picked {
		for _, v := range scripture.ByTopic(s.cfg.Vault.Root, topic) {
			key := v.Source + "|" + v.Text
			if verseSeen[key] {
				continue
			}
			verseSeen[key] = true
			verses = append(verses, v)
		}
	}
	// Stable ordering — verses with the first-picked topic come first,
	// then second-picked, then third. Within a topic, preserve the
	// catalogue order (already returned in that order by ByTopic).
	// Cap to limit so a popular topic doesn't dominate.
	if len(verses) > limit {
		verses = verses[:limit]
	}

	writeJSON(w, http.StatusOK, semanticSearchResponse{
		Topics:    picked,
		Scriptures: verses,
		Query:     query,
	})
}
