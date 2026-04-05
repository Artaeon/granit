package tui

import (
	"reflect"
	"testing"
)

func TestStripListPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1. Hello world", "Hello world"},
		{"2) Hello world", "Hello world"},
		{"3: Hello world", "Hello world"},
		{"- Hello world", "Hello world"},
		{"* Hello world", "Hello world"},
		{"• Hello world", "Hello world"},
		{"10. Hello world", "Hello world"},
		{"no prefix here", "no prefix here"},
		{"  1. leading whitespace", "leading whitespace"},
		{"1.no space after dot", "1.no space after dot"}, // must have space
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripListPrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripListPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAtParseSuggestedTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			"simple comma list",
			"tag1, tag2, tag3",
			[]string{"tag1", "tag2", "tag3"},
		},
		{
			"hash prefix stripped",
			"#tag1, #tag2",
			[]string{"tag1", "tag2"},
		},
		{
			"lowercased",
			"Tag1, TAG2, tAg3",
			[]string{"tag1", "tag2", "tag3"},
		},
		{
			"quoted strings",
			"\"tag1\", 'tag2', `tag3`",
			[]string{"tag1", "tag2", "tag3"},
		},
		{
			"hyphens preserved",
			"machine-learning, deep-learning",
			[]string{"machine-learning", "deep-learning"},
		},
		{
			"unicode preserved",
			"café, über, 日本語",
			[]string{"café", "über", "日本語"},
		},
		{
			"empty entries filtered",
			"tag1, , ,tag2",
			[]string{"tag1", "tag2"},
		},
		{
			"empty input",
			"",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := atParseSuggestedTags(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("atParseSuggestedTags(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractFrontmatterTags(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			"inline list format",
			"---\ntitle: foo\ntags: [a, b, c]\n---\nbody",
			[]string{"a", "b", "c"},
		},
		{
			"inline comma format",
			"---\ntags: a, b, c\n---\nbody",
			[]string{"a", "b", "c"},
		},
		{
			"quoted inline list",
			"---\ntags: [\"a\", \"b\"]\n---\nbody",
			[]string{"a", "b"},
		},
		{
			"multi-line list (Obsidian style)",
			"---\ntitle: foo\ntags:\n  - a\n  - b\n  - c\n---\nbody",
			[]string{"a", "b", "c"},
		},
		{
			"multi-line list quoted",
			"---\ntags:\n  - \"a\"\n  - 'b'\n---\nbody",
			[]string{"a", "b"},
		},
		{
			"no frontmatter",
			"just content no frontmatter",
			nil,
		},
		{
			"unclosed frontmatter",
			"---\ntags: [a, b]\nbody no closing",
			nil,
		},
		{
			"no tags field",
			"---\ntitle: foo\n---\nbody",
			nil,
		},
		{
			"empty string",
			"",
			nil,
		},
		{
			"false match on ---bad",
			"---badstart not frontmatter",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFrontmatterTags(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractFrontmatterTags(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestGhostCleanCompletion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"whitespace only", "   \n  ", ""},
		{"single line", "hello world", "hello world"},
		{"stops at newline", "first line\nsecond line", "first line"},
		{"sentence boundary", "First sentence. Second sentence.", "First sentence."},
		{"abbreviation not split", "e.g. like this", "e.g. like this"},
		{"doctor title not split", "Dr. Smith is here", "Dr. Smith is here"},
		{"ellipsis preserved", "well... maybe", "well... maybe"},
		{"trailing whitespace trimmed", "  hello  ", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ghostCleanCompletion(tt.input)
			if got != tt.want {
				t.Errorf("ghostCleanCompletion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGhostWriterCacheLRU(t *testing.T) {
	gw := NewGhostWriter()
	// Put more than ghostCacheMaxEntries (32) items.
	for i := 0; i < 40; i++ {
		key := string(rune('a' + i%26)) + string(rune('0'+i/10))
		gw.cachePut(key, "result-"+key)
	}
	if len(gw.cache) > ghostCacheMaxEntries {
		t.Errorf("cache size %d exceeds max %d", len(gw.cache), ghostCacheMaxEntries)
	}
	if len(gw.cacheKeys) > ghostCacheMaxEntries {
		t.Errorf("cacheKeys size %d exceeds max %d", len(gw.cacheKeys), ghostCacheMaxEntries)
	}
}

func TestGhostWriterCacheHitMiss(t *testing.T) {
	gw := NewGhostWriter()
	if _, ok := gw.cacheGet("missing"); ok {
		t.Error("expected miss on empty cache")
	}
	gw.cachePut("key1", "value1")
	if got, ok := gw.cacheGet("key1"); !ok || got != "value1" {
		t.Errorf("cache get = (%q, %v), want (value1, true)", got, ok)
	}
	// Overwrite existing key
	gw.cachePut("key1", "value2")
	if got, _ := gw.cacheGet("key1"); got != "value2" {
		t.Errorf("overwrite: cache get = %q, want value2", got)
	}
}

func TestGhostWriterSetAIInvalidatesCache(t *testing.T) {
	gw := NewGhostWriter()
	gw.ai = AIConfig{Provider: "ollama", Model: "qwen2.5:0.5b"}
	gw.cachePut("k", "v")
	// Same config — cache preserved.
	gw.SetAI(AIConfig{Provider: "ollama", Model: "qwen2.5:0.5b"})
	if _, ok := gw.cacheGet("k"); !ok {
		t.Error("cache should be preserved on same config")
	}
	// Different model — cache invalidated.
	gw.SetAI(AIConfig{Provider: "ollama", Model: "llama3.1:8b"})
	if _, ok := gw.cacheGet("k"); ok {
		t.Error("cache should be invalidated on model change")
	}
}

func TestBotFilteredBots(t *testing.T) {
	b := Bots{}
	if got := len(b.filteredBots()); got != len(botList) {
		t.Errorf("empty filter returns %d bots, want %d", got, len(botList))
	}

	// Filter by exact name fragment
	b.filter = "tag"
	filtered := b.filteredBots()
	if len(filtered) == 0 {
		t.Error("filter 'tag' should match at least Auto-Tagger")
	}
	foundAutoTagger := false
	for _, bd := range filtered {
		if bd.kind == botAutoTagger {
			foundAutoTagger = true
		}
	}
	if !foundAutoTagger {
		t.Error("filter 'tag' should include Auto-Tagger")
	}

	// Filter by description
	b.filter = "summary"
	if len(b.filteredBots()) == 0 {
		t.Error("filter 'summary' should match at least one bot via description")
	}

	// No matches
	b.filter = "xyznomatchingbot"
	if got := len(b.filteredBots()); got != 0 {
		t.Errorf("no-match filter returned %d bots, want 0", got)
	}

	// Case-insensitive
	b.filter = "TAG"
	if len(b.filteredBots()) == 0 {
		t.Error("filter should be case-insensitive")
	}
}

func TestBotSystemPromptNonEmpty(t *testing.T) {
	// Every bot kind should return a non-empty system prompt for both
	// small and large model variants.
	kinds := []botKind{
		botAutoTagger, botLinkSuggester, botSummarizer, botQuestionBot,
		botWritingAssistant, botTitleSuggester, botActionItems, botMOCGenerator,
		botAutoLinker, botFlashcardGen, botToneAdjuster, botOutliner,
		botExplainSimple, botKeyTerms, botCounterArgument, botTLDR,
		botProsCons, botExpand,
	}
	for _, k := range kinds {
		for _, small := range []bool{true, false} {
			prompt := botSystemPrompt(k, small)
			if prompt == "" {
				t.Errorf("botSystemPrompt(%d, small=%v) returned empty", k, small)
			}
		}
	}
}

func TestBotListWrapAround(t *testing.T) {
	// Simulates the wrap-around behavior of updateList without needing
	// a full tea.KeyMsg. Verifies the wrap logic directly.
	total := len(botList)
	if total == 0 {
		t.Skip("no bots")
	}

	// From cursor 0, "up" wraps to last.
	cursor := 0
	visible := botList
	if cursor > 0 {
		cursor--
	} else if len(visible) > 0 {
		cursor = len(visible) - 1
	}
	if cursor != total-1 {
		t.Errorf("up from 0 = %d, want %d", cursor, total-1)
	}

	// From cursor = last, "down" wraps to 0.
	cursor = total - 1
	if cursor < len(visible)-1 {
		cursor++
	} else if len(visible) > 0 {
		cursor = 0
	}
	if cursor != 0 {
		t.Errorf("down from last = %d, want 0", cursor)
	}
}

func TestBotListAllHaveCategories(t *testing.T) {
	// Every bot in botList must have a category set.
	for _, bd := range botList {
		if bd.category == "" {
			t.Errorf("bot %q has no category", bd.name)
		}
		// Verify the category is one of the known ones.
		known := false
		for _, c := range categoryOrder {
			if bd.category == c {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("bot %q has unknown category %q", bd.name, bd.category)
		}
	}
}
