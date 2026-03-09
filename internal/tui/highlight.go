package tui

import (
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

// chromaCache caches highlighted output for code blocks. The key is
// language + "\x00" + source text; the value is the pre-rendered styled
// string per line.
var chromaCache struct {
	sync.Mutex
	entries map[string][]string
}

func init() {
	chromaCache.entries = make(map[string][]string)
}

// InvalidateChromaCache clears the entire highlight cache.
// Call this when the theme changes.
func InvalidateChromaCache() {
	chromaCache.Lock()
	chromaCache.entries = make(map[string][]string)
	chromaCache.Unlock()
}

// chromaCacheKey builds a cache key from language and source lines.
func chromaCacheKey(lang string, lines []string) string {
	var b strings.Builder
	b.WriteString(lang)
	b.WriteByte(0)
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(l)
	}
	return b.String()
}

// HighlightCodeBlock takes a language identifier and lines of source code,
// returning one styled string per input line. Results are cached.
func HighlightCodeBlock(lang string, lines []string) []string {
	key := chromaCacheKey(lang, lines)

	chromaCache.Lock()
	if cached, ok := chromaCache.entries[key]; ok {
		chromaCache.Unlock()
		return cached
	}
	chromaCache.Unlock()

	result := highlightWithChroma(lang, lines)

	chromaCache.Lock()
	// Cap cache size to avoid unbounded growth.
	if len(chromaCache.entries) > 512 {
		chromaCache.entries = make(map[string][]string)
	}
	chromaCache.entries[key] = result
	chromaCache.Unlock()

	return result
}

// HighlightCodeLine highlights a single line using chroma. This is a
// convenience wrapper around HighlightCodeBlock for the editor, which
// processes one line at a time.
func HighlightCodeLine(lang string, line string) string {
	if lang == "" {
		return renderFallbackCodeLine(line)
	}

	lexer := findLexer(lang)
	if lexer == nil {
		return renderFallbackCodeLine(line)
	}

	tokens := tokenizeChroma(lexer, line)
	return renderChromaTokens(tokens)
}

// findLexer resolves a language string to a chroma lexer.
func findLexer(lang string) chroma.Lexer {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return nil
	}
	l := lexers.Get(lang)
	if l == nil {
		return nil
	}
	return chroma.Coalesce(l)
}

// tokenizeChroma runs the chroma lexer on a single line and returns the
// resulting token stream.
func tokenizeChroma(lexer chroma.Lexer, line string) []chroma.Token {
	iter, err := lexer.Tokenise(nil, line)
	if err != nil {
		return []chroma.Token{{Type: chroma.Text, Value: line}}
	}
	var tokens []chroma.Token
	for _, t := range iter.Tokens() {
		// Strip trailing newlines that chroma may insert.
		t.Value = strings.TrimRight(t.Value, "\n")
		if t.Value != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// renderChromaTokens converts chroma tokens into a lipgloss-styled string,
// using the current Granit theme colors.
func renderChromaTokens(tokens []chroma.Token) string {
	var b strings.Builder
	for _, t := range tokens {
		style := chromaTokenStyle(t.Type)
		b.WriteString(style.Render(t.Value))
	}
	return b.String()
}

// chromaTokenStyle maps a chroma token type to a lipgloss style using the
// current theme variables from styles.go.
func chromaTokenStyle(tt chroma.TokenType) lipgloss.Style {
	s := lipgloss.NewStyle().Background(surface0)

	switch {
	// Keywords
	case tt == chroma.Keyword,
		tt == chroma.KeywordConstant,
		tt == chroma.KeywordDeclaration,
		tt == chroma.KeywordNamespace,
		tt == chroma.KeywordPseudo,
		tt == chroma.KeywordReserved,
		tt == chroma.KeywordType:
		return s.Foreground(mauve).Bold(true)

	// Names / functions
	case tt == chroma.NameFunction,
		tt == chroma.NameFunctionMagic,
		tt == chroma.NameBuiltin,
		tt == chroma.NameBuiltinPseudo:
		return s.Foreground(blue)

	// Types / classes
	case tt == chroma.NameClass,
		tt == chroma.NameException,
		tt == chroma.NameDecorator,
		tt == chroma.NameEntity:
		return s.Foreground(yellow)

	// Strings
	case tt == chroma.LiteralString,
		tt == chroma.LiteralStringAffix,
		tt == chroma.LiteralStringBacktick,
		tt == chroma.LiteralStringChar,
		tt == chroma.LiteralStringDelimiter,
		tt == chroma.LiteralStringDoc,
		tt == chroma.LiteralStringDouble,
		tt == chroma.LiteralStringEscape,
		tt == chroma.LiteralStringHeredoc,
		tt == chroma.LiteralStringInterpol,
		tt == chroma.LiteralStringOther,
		tt == chroma.LiteralStringRegex,
		tt == chroma.LiteralStringSingle,
		tt == chroma.LiteralStringSymbol:
		return s.Foreground(green)

	// Numbers
	case tt == chroma.LiteralNumber,
		tt == chroma.LiteralNumberBin,
		tt == chroma.LiteralNumberFloat,
		tt == chroma.LiteralNumberHex,
		tt == chroma.LiteralNumberInteger,
		tt == chroma.LiteralNumberIntegerLong,
		tt == chroma.LiteralNumberOct:
		return s.Foreground(peach)

	// Comments
	case tt == chroma.Comment,
		tt == chroma.CommentHashbang,
		tt == chroma.CommentMultiline,
		tt == chroma.CommentPreproc,
		tt == chroma.CommentPreprocFile,
		tt == chroma.CommentSingle,
		tt == chroma.CommentSpecial:
		return s.Foreground(overlay0).Italic(true)

	// Operators
	case tt == chroma.Operator,
		tt == chroma.OperatorWord:
		return s.Foreground(sky)

	// Punctuation
	case tt == chroma.Punctuation:
		return s.Foreground(overlay2)

	// Name attributes, tags (common in HTML/XML/CSS)
	case tt == chroma.NameAttribute,
		tt == chroma.NameTag:
		return s.Foreground(blue)

	// Name variables
	case tt == chroma.NameVariable,
		tt == chroma.NameVariableClass,
		tt == chroma.NameVariableGlobal,
		tt == chroma.NameVariableInstance,
		tt == chroma.NameVariableMagic:
		return s.Foreground(peach)

	// Name constants
	case tt == chroma.NameConstant:
		return s.Foreground(peach)

	// Name other / labels
	case tt == chroma.NameLabel,
		tt == chroma.NameNamespace:
		return s.Foreground(yellow)

	// Generic emphasis / strong (used in diffs, etc.)
	case tt == chroma.GenericEmph:
		return s.Italic(true).Foreground(text)
	case tt == chroma.GenericStrong:
		return s.Bold(true).Foreground(text)
	case tt == chroma.GenericInserted:
		return s.Foreground(green)
	case tt == chroma.GenericDeleted:
		return s.Foreground(red)
	case tt == chroma.GenericHeading,
		tt == chroma.GenericSubheading:
		return s.Foreground(blue).Bold(true)

	default:
		return s.Foreground(text)
	}
}

// renderFallbackCodeLine renders a code line with the basic code block style
// when no language is detected or the language is unknown to chroma.
func renderFallbackCodeLine(line string) string {
	return CodeBlockStyle.Render(line)
}

// highlightWithChroma highlights multiple lines using chroma, returning one
// styled string per line.
func highlightWithChroma(lang string, lines []string) []string {
	if lang == "" {
		result := make([]string, len(lines))
		for i, l := range lines {
			result[i] = renderFallbackCodeLine(l)
		}
		return result
	}

	lexer := findLexer(lang)
	if lexer == nil {
		result := make([]string, len(lines))
		for i, l := range lines {
			result[i] = renderFallbackCodeLine(l)
		}
		return result
	}

	// Tokenize the entire block at once for correct multi-line state
	// (e.g. multi-line strings, block comments).
	source := strings.Join(lines, "\n")
	iter, err := lexer.Tokenise(nil, source)
	if err != nil {
		result := make([]string, len(lines))
		for i, l := range lines {
			result[i] = renderFallbackCodeLine(l)
		}
		return result
	}

	// Split token stream back into per-line output.
	result := make([]string, len(lines))
	lineIdx := 0
	var b strings.Builder

	for _, tok := range iter.Tokens() {
		// A token may span multiple lines (e.g. multi-line strings).
		parts := strings.Split(tok.Value, "\n")
		for pi, part := range parts {
			if pi > 0 {
				// Line break within token — finish current line, advance.
				if lineIdx < len(result) {
					result[lineIdx] = b.String()
				}
				b.Reset()
				lineIdx++
				if lineIdx >= len(lines) {
					break
				}
			}
			if part != "" {
				style := chromaTokenStyle(tok.Type)
				b.WriteString(style.Render(part))
			}
		}
		if lineIdx >= len(lines) {
			break
		}
	}
	// Flush the last line.
	if lineIdx < len(result) {
		result[lineIdx] = b.String()
	}

	return result
}
