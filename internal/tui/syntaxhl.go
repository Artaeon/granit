package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// Language definitions for syntax highlighting in fenced code blocks.

type langDef struct {
	keywords    map[string]bool
	types       map[string]bool
	lineComment string
	blockStart  string
	blockEnd    string
	hashComment bool // uses # for comments
	shellVars   bool // highlight $VAR and ${VAR}
}

var langDefs = map[string]*langDef{
	"go": {
		keywords: toSet([]string{
			"func", "return", "if", "for", "range", "var", "const", "type",
			"struct", "interface", "package", "import", "defer", "go", "chan",
			"select", "switch", "case", "default", "map", "make", "nil",
			"true", "false", "err", "else", "break", "continue", "fallthrough",
			"goto",
		}),
		types: toSet([]string{
			"string", "int", "bool", "error", "byte", "float64", "float32",
			"int8", "int16", "int32", "int64", "uint", "uint8", "uint16",
			"uint32", "uint64", "rune", "uintptr", "complex64", "complex128",
		}),
		lineComment: "//",
		blockStart:  "/*",
		blockEnd:    "*/",
	},
	"python": {
		keywords: toSet([]string{
			"def", "class", "return", "if", "elif", "else", "for", "while",
			"import", "from", "with", "as", "try", "except", "raise", "None",
			"True", "False", "self", "lambda", "yield", "async", "await",
			"in", "not", "and", "or", "is", "pass", "break", "continue",
			"del", "global", "nonlocal", "assert", "finally",
		}),
		types:       toSet([]string{}),
		hashComment: true,
	},
	"javascript": {
		keywords: toSet([]string{
			"function", "const", "let", "var", "return", "if", "else", "for",
			"while", "class", "import", "export", "from", "async", "await",
			"new", "this", "null", "undefined", "true", "false", "typeof",
			"switch", "case", "default", "break", "continue", "throw", "try",
			"catch", "finally", "of", "in", "instanceof", "delete", "void",
			"yield", "super", "extends", "static",
		}),
		types:       toSet([]string{}),
		lineComment: "//",
		blockStart:  "/*",
		blockEnd:    "*/",
	},
	"rust": {
		keywords: toSet([]string{
			"fn", "let", "mut", "pub", "struct", "impl", "enum", "match",
			"use", "mod", "return", "if", "else", "for", "while", "loop",
			"self", "Self", "Some", "None", "Ok", "Err", "true", "false",
			"unsafe", "async", "await", "trait", "where", "const", "static",
			"ref", "move", "break", "continue", "as", "in", "extern",
			"crate", "super", "type", "dyn", "macro_rules",
		}),
		types: toSet([]string{
			"i8", "i16", "i32", "i64", "i128", "isize",
			"u8", "u16", "u32", "u64", "u128", "usize",
			"f32", "f64", "bool", "str", "String", "Vec",
			"Option", "Result", "Box", "Rc", "Arc", "HashMap",
			"char",
		}),
		lineComment: "//",
		blockStart:  "/*",
		blockEnd:    "*/",
	},
	"shell": {
		keywords: toSet([]string{
			"if", "then", "else", "elif", "fi", "for", "do", "done", "while",
			"case", "esac", "function", "return", "export", "echo", "cd", "ls",
			"in", "select", "until", "local", "readonly", "declare", "set",
			"unset", "shift", "source", "exit", "eval", "exec", "trap",
		}),
		types:       toSet([]string{}),
		hashComment: true,
		shellVars:   true,
	},
}

// Language aliases map common fence names to canonical definitions.
var langAliases = map[string]string{
	"golang":     "go",
	"py":         "python",
	"python3":    "python",
	"js":         "javascript",
	"jsx":        "javascript",
	"ts":         "javascript",
	"tsx":        "javascript",
	"typescript": "javascript",
	"rs":         "rust",
	"sh":         "shell",
	"bash":       "shell",
	"zsh":        "shell",
	"fish":       "shell",
	"ksh":        "shell",
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}

// resolveLang returns the canonical language definition for a fence language tag.
// Returns nil if the language is unknown (will use generic fallback).
func resolveLang(lang string) *langDef {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if alias, ok := langAliases[lang]; ok {
		lang = alias
	}
	return langDefs[lang]
}

// tokenKind identifies the type of a syntax token for coloring.
type tokenKind int

const (
	tkNormal tokenKind = iota
	tkKeyword
	tkType
	tkString
	tkComment
	tkNumber
	tkFunction
	tkOperator
	tkShellVar
)

// token represents a colored fragment of a code line.
type token struct {
	text string
	kind tokenKind
}

// highlightCodeLine returns a syntax-highlighted rendering of a single code line.
// lang is the language identifier from the opening fence (e.g. "go", "python").
// If lang is empty or unknown, a generic fallback highlighting strings and comments.
func highlightCodeLine(line string, lang string) string {
	ld := resolveLang(lang)
	tokens := tokenizeLine(line, ld)
	return renderTokens(tokens)
}

// renderTokens converts a slice of tokens into a styled string using theme colors.
func renderTokens(tokens []token) string {
	var b strings.Builder
	for _, t := range tokens {
		style := lipgloss.NewStyle().Background(surface0)
		switch t.kind {
		case tkKeyword:
			style = style.Foreground(mauve).Bold(true)
		case tkType:
			style = style.Foreground(yellow)
		case tkString:
			style = style.Foreground(green)
		case tkComment:
			style = style.Foreground(overlay0).Italic(true)
		case tkNumber:
			style = style.Foreground(peach)
		case tkFunction:
			style = style.Foreground(blue)
		case tkOperator:
			style = style.Foreground(sky)
		case tkShellVar:
			style = style.Foreground(peach)
		default:
			style = style.Foreground(text)
		}
		b.WriteString(style.Render(t.text))
	}
	return b.String()
}

// tokenizeLine breaks a code line into tokens according to the language definition.
// If ld is nil, generic fallback tokenization is used (strings + comments only).
func tokenizeLine(line string, ld *langDef) []token {
	if ld == nil {
		return tokenizeGeneric(line)
	}

	var tokens []token
	runes := []rune(line)
	n := len(runes)
	i := 0

	for i < n {
		// --- Line comment ---
		if ld.lineComment != "" && i+len(ld.lineComment) <= n {
			if string(runes[i:i+len(ld.lineComment)]) == ld.lineComment {
				tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
				return tokens
			}
		}

		// --- Hash comment ---
		if ld.hashComment && runes[i] == '#' {
			tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
			return tokens
		}

		// --- Block comment (single-line portion) ---
		if ld.blockStart != "" && i+len(ld.blockStart) <= n {
			if string(runes[i:i+len(ld.blockStart)]) == ld.blockStart {
				endIdx := indexRunes(runes, i+len(ld.blockStart), ld.blockEnd)
				if endIdx >= 0 {
					end := endIdx + len(ld.blockEnd)
					tokens = append(tokens, token{text: string(runes[i:end]), kind: tkComment})
					i = end
					continue
				}
				// No closing — rest of line is comment
				tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
				return tokens
			}
		}

		// --- Shell variables ---
		if ld.shellVars && runes[i] == '$' {
			tok, advance := readShellVar(runes, i)
			if advance > 0 {
				tokens = append(tokens, token{text: tok, kind: tkShellVar})
				i += advance
				continue
			}
		}

		// --- Strings (double-quote, single-quote, backtick) ---
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			tok, advance := readString(runes, i)
			tokens = append(tokens, token{text: tok, kind: tkString})
			i += advance
			continue
		}

		// --- Numbers ---
		if isDigitStart(runes, i) {
			tok, advance := readNumber(runes, i)
			tokens = append(tokens, token{text: tok, kind: tkNumber})
			i += advance
			continue
		}

		// --- Identifiers / keywords / types ---
		if isIdentStart(runes[i]) {
			tok, advance := readIdent(runes, i)
			kind := tkNormal
			if ld.keywords[tok] {
				kind = tkKeyword
			} else if ld.types[tok] {
				kind = tkType
			} else if i+advance < n && runes[i+advance] == '(' {
				kind = tkFunction
			}
			tokens = append(tokens, token{text: tok, kind: kind})
			i += advance
			continue
		}

		// --- Arrow operator => ---
		if runes[i] == '=' && i+1 < n && runes[i+1] == '>' {
			tokens = append(tokens, token{text: "=>", kind: tkOperator})
			i += 2
			continue
		}

		// --- Operators ---
		if isOperator(runes[i]) {
			tokens = append(tokens, token{text: string(runes[i]), kind: tkOperator})
			i++
			continue
		}

		// --- Whitespace and other characters ---
		j := i
		for j < n && !isTokenStart(runes[j], ld) {
			j++
		}
		if j > i {
			tokens = append(tokens, token{text: string(runes[i:j]), kind: tkNormal})
			i = j
		} else {
			tokens = append(tokens, token{text: string(runes[i]), kind: tkNormal})
			i++
		}
	}

	return tokens
}

// tokenizeGeneric handles unknown languages: only strings and C-style / hash comments.
func tokenizeGeneric(line string) []token {
	var tokens []token
	runes := []rune(line)
	n := len(runes)
	i := 0

	for i < n {
		// Line comment //
		if i+2 <= n && runes[i] == '/' && runes[i+1] == '/' {
			tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
			return tokens
		}
		// Hash comment
		if runes[i] == '#' {
			tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
			return tokens
		}
		// Block comment
		if i+2 <= n && runes[i] == '/' && runes[i+1] == '*' {
			endIdx := indexRunes(runes, i+2, "*/")
			if endIdx >= 0 {
				end := endIdx + 2
				tokens = append(tokens, token{text: string(runes[i:end]), kind: tkComment})
				i = end
				continue
			}
			tokens = append(tokens, token{text: string(runes[i:]), kind: tkComment})
			return tokens
		}
		// Strings
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			tok, advance := readString(runes, i)
			tokens = append(tokens, token{text: tok, kind: tkString})
			i += advance
			continue
		}
		// Numbers
		if isDigitStart(runes, i) {
			tok, advance := readNumber(runes, i)
			tokens = append(tokens, token{text: tok, kind: tkNumber})
			i += advance
			continue
		}
		// Normal text
		j := i + 1
		for j < n && runes[j] != '/' && runes[j] != '#' && runes[j] != '"' && runes[j] != '\'' && runes[j] != '`' && !isDigitStart(runes, j) {
			j++
		}
		tokens = append(tokens, token{text: string(runes[i:j]), kind: tkNormal})
		i = j
	}

	return tokens
}

// isTokenStart returns true if the rune could begin a significant token.
func isTokenStart(r rune, ld *langDef) bool {
	if r == '"' || r == '\'' || r == '`' {
		return true
	}
	if isIdentStart(r) || unicode.IsDigit(r) || isOperator(r) {
		return true
	}
	if ld.lineComment != "" && r == rune(ld.lineComment[0]) {
		return true
	}
	if ld.hashComment && r == '#' {
		return true
	}
	if ld.blockStart != "" && r == rune(ld.blockStart[0]) {
		return true
	}
	if ld.shellVars && r == '$' {
		return true
	}
	return false
}

// readString reads a quoted string from runes[i], handling escape sequences.
func readString(runes []rune, i int) (string, int) {
	quote := runes[i]
	j := i + 1
	n := len(runes)
	for j < n {
		if runes[j] == '\\' && j+1 < n {
			j += 2 // skip escaped char
			continue
		}
		if runes[j] == quote {
			j++
			return string(runes[i:j]), j - i
		}
		j++
	}
	// Unterminated string — take the rest
	return string(runes[i:]), n - i
}

// readNumber reads a numeric literal (int, float, hex).
func readNumber(runes []rune, i int) (string, int) {
	j := i
	n := len(runes)

	// Hex prefix
	if j+2 < n && runes[j] == '0' && (runes[j+1] == 'x' || runes[j+1] == 'X') {
		j += 2
		for j < n && isHexDigit(runes[j]) {
			j++
		}
		return string(runes[i:j]), j - i
	}

	// Decimal / float
	for j < n && unicode.IsDigit(runes[j]) {
		j++
	}
	if j < n && runes[j] == '.' && j+1 < n && unicode.IsDigit(runes[j+1]) {
		j++
		for j < n && unicode.IsDigit(runes[j]) {
			j++
		}
	}
	// Exponent
	if j < n && (runes[j] == 'e' || runes[j] == 'E') {
		j++
		if j < n && (runes[j] == '+' || runes[j] == '-') {
			j++
		}
		for j < n && unicode.IsDigit(runes[j]) {
			j++
		}
	}

	if j == i {
		return string(runes[i : i+1]), 1
	}
	return string(runes[i:j]), j - i
}

// readIdent reads an identifier (word).
func readIdent(runes []rune, i int) (string, int) {
	j := i
	n := len(runes)
	for j < n && isIdentContinue(runes[j]) {
		j++
	}
	return string(runes[i:j]), j - i
}

// readShellVar reads a shell variable starting at $.
func readShellVar(runes []rune, i int) (string, int) {
	n := len(runes)
	if i+1 >= n {
		return "", 0
	}
	// ${VAR}
	if runes[i+1] == '{' {
		j := i + 2
		for j < n && runes[j] != '}' {
			j++
		}
		if j < n {
			j++ // include closing }
		}
		return string(runes[i:j]), j - i
	}
	// $VAR
	if isIdentStart(runes[i+1]) || unicode.IsDigit(runes[i+1]) {
		j := i + 1
		for j < n && isIdentContinue(runes[j]) {
			j++
		}
		return string(runes[i:j]), j - i
	}
	// Special vars: $?, $!, $@, $#, $$, $*
	if i+1 < n && strings.ContainsRune("?!@#$*-0", runes[i+1]) {
		return string(runes[i : i+2]), 2
	}
	return "", 0
}

// indexRunes finds the first occurrence of needle in runes starting at position start.
func indexRunes(runes []rune, start int, needle string) int {
	needleRunes := []rune(needle)
	nLen := len(needleRunes)
	n := len(runes)
	for i := start; i+nLen <= n; i++ {
		match := true
		for j := 0; j < nLen; j++ {
			if runes[i+j] != needleRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentContinue(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isDigitStart(runes []rune, i int) bool {
	if i >= len(runes) {
		return false
	}
	r := runes[i]
	if unicode.IsDigit(r) {
		// Make sure it's not part of an identifier
		if i > 0 && isIdentContinue(runes[i-1]) {
			return false
		}
		return true
	}
	return false
}

func isHexDigit(r rune) bool {
	return unicode.IsDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func isOperator(r rune) bool {
	return strings.ContainsRune("+-*/%=<>!&|^~?:", r)
}

// parseFenceLang extracts the language from a fenced code block opening line.
// E.g. "```go" returns "go", "```python" returns "python", "```" returns "".
func parseFenceLang(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "```") {
		return ""
	}
	lang := strings.TrimPrefix(trimmed, "```")
	lang = strings.TrimSpace(lang)
	// Remove any trailing text after whitespace (e.g. ```python title="example")
	if idx := strings.IndexByte(lang, ' '); idx >= 0 {
		lang = lang[:idx]
	}
	return strings.ToLower(lang)
}
