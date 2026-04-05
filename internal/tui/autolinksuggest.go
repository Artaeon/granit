package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// autoLinkSuggestMsg carries AI link suggestions back to the Update loop.
type autoLinkSuggestMsg struct {
	suggestions []string
	err         error
}

// AutoLinkSuggest generates AI-powered wikilink suggestions on save.
type AutoLinkSuggest struct {
	enabled  bool
	ai       AIConfig
	inFlight bool
}

// SetInFlight marks whether a request is currently running so callers can
// skip re-triggering on rapid saves.
func (als *AutoLinkSuggest) SetInFlight(v bool) { als.inFlight = v }

// IsInFlight reports whether a request is currently running.
func (als *AutoLinkSuggest) IsInFlight() bool { return als.inFlight }

// NewAutoLinkSuggest creates a new AutoLinkSuggest.
func NewAutoLinkSuggest() *AutoLinkSuggest {
	return &AutoLinkSuggest{}
}

// SetEnabled toggles AI link suggestions on save.
func (als *AutoLinkSuggest) SetEnabled(enabled bool) { als.enabled = enabled }

// IsEnabled returns whether link suggestions are enabled.
func (als *AutoLinkSuggest) IsEnabled() bool { return als.enabled }

// SuggestLinks sends note content and available note names to the AI for
// wikilink suggestions. Returns nil if disabled or no candidates.
func (als *AutoLinkSuggest) SuggestLinks(content string, noteNames []string, currentNote string) tea.Cmd {
	if !als.enabled || als.ai.Provider == "" || als.ai.Provider == "local" {
		return nil
	}
	if len(noteNames) == 0 {
		return nil
	}
	// Skip if a previous request is still running — prevents request pile-up
	// on rapid saves with slow small models.
	if als.inFlight {
		return nil
	}

	ai := als.ai

	// Truncate content for the prompt — use less for small models.
	maxRunes := 2000
	maxCandidates := 200
	if ai.IsSmallModel() {
		maxRunes = 600
		maxCandidates = 40
	}
	runes := []rune(content)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	snippet := string(runes)

	// Build candidate list (exclude current note).
	var candidates []string
	for _, n := range noteNames {
		name := strings.TrimSuffix(n, ".md")
		if n != currentNote && len(candidates) < maxCandidates {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	candidateList := strings.Join(candidates, ", ")
	systemPrompt := "You are a note connection assistant. Given a note's content and a list of other note names, suggest 1-3 notes that should be linked with [[wikilinks]]. Return ONLY a comma-separated list of note names, nothing else. If no good links exist, return NONE."
	userPrompt := "NOTE CONTENT:\n" + snippet + "\n\nAVAILABLE NOTES:\n" + candidateList

	// Skip if the prompt wouldn't fit in the model's context.
	if fits, _, _ := ai.PromptFitsContext(systemPrompt, userPrompt); !fits {
		return nil
	}

	als.inFlight = true

	return func() tea.Msg {
		deadline := 45 * time.Second
		if ai.IsSmallModel() {
			deadline = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		resp, err := ai.ChatShortCtx(ctx, systemPrompt, userPrompt)
		if err != nil {
			return autoLinkSuggestMsg{err: err}
		}

		resp = strings.TrimSpace(resp)
		if resp == "" || strings.EqualFold(resp, "NONE") {
			return autoLinkSuggestMsg{}
		}

		var suggestions []string
		for _, s := range strings.Split(resp, ",") {
			s = strings.TrimSpace(s)
			s = strings.Trim(s, "[]\"'`")
			if s != "" && !strings.EqualFold(s, "NONE") {
				suggestions = append(suggestions, s)
			}
		}
		if len(suggestions) > 3 {
			suggestions = suggestions[:3]
		}
		return autoLinkSuggestMsg{suggestions: suggestions}
	}
}
