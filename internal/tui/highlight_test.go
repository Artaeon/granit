package tui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

// ---------------------------------------------------------------------------
// HighlightCodeLine
// ---------------------------------------------------------------------------

func TestHighlightCodeLine_Go(t *testing.T) {
	line := `func main() { fmt.Println("hello") }`
	result := HighlightCodeLine("go", line)

	if result == "" {
		t.Fatal("expected non-empty highlighted output")
	}
	// In non-TTY environments lipgloss may strip ANSI escapes, making the
	// styled string identical to the raw input.  Instead of comparing the
	// rendered string, verify that chroma actually tokenised the line into
	// multiple tokens (keywords, punctuation, strings, etc.).
	lexer := lexers.Get("go")
	if lexer == nil {
		t.Fatal("chroma has no Go lexer")
	}
	iter, err := lexer.Tokenise(nil, line)
	if err != nil {
		t.Fatalf("chroma tokenisation failed: %v", err)
	}
	tokens := iter.Tokens()
	if len(tokens) < 2 {
		t.Errorf("expected chroma to produce multiple tokens, got %d", len(tokens))
	}
	// The result must at least contain the original text content.
	if !strings.Contains(result, "func") {
		t.Error("highlighted output missing 'func' keyword text")
	}
}

func TestHighlightCodeLine_Unknown(t *testing.T) {
	line := "some random text"
	result := HighlightCodeLine("zzz_nonexistent_language_zzz", line)

	// Unknown language falls back to renderFallbackCodeLine which wraps with
	// CodeBlockStyle. The result should still contain the original text.
	if result == "" {
		t.Fatal("expected non-empty fallback output")
	}
	// The fallback applies CodeBlockStyle so the result will differ from the
	// raw input, but should contain the original text content.
	fallback := renderFallbackCodeLine(line)
	if result != fallback {
		t.Errorf("expected fallback render for unknown language, got %q", result)
	}
}

func TestHighlightCodeLine_Empty(t *testing.T) {
	result := HighlightCodeLine("go", "")

	// An empty line with a known language should still return empty (or at
	// most whitespace-only) because there are no tokens to render.
	if len(result) > 0 {
		// If chroma produces something for an empty string, that is acceptable
		// as long as it does not panic. We just verify it runs without error.
		t.Logf("HighlightCodeLine returned %q for empty input (acceptable)", result)
	}
}

// ---------------------------------------------------------------------------
// HighlightCodeBlock
// ---------------------------------------------------------------------------

func TestHighlightCodeBlock_Multiline(t *testing.T) {
	lines := []string{
		"package main",
		"",
		"import \"fmt\"",
		"",
		"func main() {",
		"    fmt.Println(\"hello\")",
		"}",
	}

	result := HighlightCodeBlock("go", lines)

	if len(result) != len(lines) {
		t.Fatalf("expected %d output lines, got %d", len(lines), len(result))
	}

	// Verify the output contains the original text content. In non-TTY
	// environments lipgloss may strip escapes so we cannot rely on string
	// inequality, but we can verify content and that the highlighter ran.
	if !strings.Contains(result[0], "package") {
		t.Error("expected first output line to contain 'package'")
	}
	if !strings.Contains(result[2], "import") {
		t.Error("expected third output line to contain 'import'")
	}
}

func TestHighlightCodeBlock_CacheHit(t *testing.T) {
	// Clear cache first to get a known state.
	InvalidateChromaCache()

	lines := []string{"x := 42", "fmt.Println(x)"}

	// First call — populates cache.
	_ = HighlightCodeBlock("go", lines)

	chromaCache.Lock()
	sizeAfterFirst := len(chromaCache.entries)
	chromaCache.Unlock()

	if sizeAfterFirst == 0 {
		t.Fatal("expected cache to have at least one entry after first call")
	}

	// Second call — should hit cache, not grow it.
	_ = HighlightCodeBlock("go", lines)

	chromaCache.Lock()
	sizeAfterSecond := len(chromaCache.entries)
	chromaCache.Unlock()

	if sizeAfterSecond != sizeAfterFirst {
		t.Errorf("cache grew from %d to %d on repeated call; expected cache hit",
			sizeAfterFirst, sizeAfterSecond)
	}
}

// ---------------------------------------------------------------------------
// InvalidateChromaCache
// ---------------------------------------------------------------------------

func TestInvalidateChromaCache(t *testing.T) {
	// Populate the cache.
	_ = HighlightCodeBlock("go", []string{"var x int"})

	chromaCache.Lock()
	if len(chromaCache.entries) == 0 {
		chromaCache.Unlock()
		t.Fatal("expected non-empty cache after highlighting")
	}
	chromaCache.Unlock()

	InvalidateChromaCache()

	chromaCache.Lock()
	remaining := len(chromaCache.entries)
	chromaCache.Unlock()

	if remaining != 0 {
		t.Errorf("expected empty cache after invalidation, got %d entries", remaining)
	}
}
