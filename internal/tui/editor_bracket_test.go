package tui

import "testing"

// ---------------------------------------------------------------------------
// isBracketChar / isOpenBracket
// ---------------------------------------------------------------------------

func TestIsBracketChar(t *testing.T) {
	for _, ch := range []byte{'(', ')', '[', ']', '{', '}'} {
		if !isBracketChar(ch) {
			t.Errorf("expected %c to be a bracket char", ch)
		}
	}
	for _, ch := range []byte{'a', '0', ' ', '"', '<', '>'} {
		if isBracketChar(ch) {
			t.Errorf("expected %c NOT to be a bracket char", ch)
		}
	}
}

func TestIsOpenBracket(t *testing.T) {
	for _, ch := range []byte{'(', '[', '{'} {
		if !isOpenBracket(ch) {
			t.Errorf("expected %c to be an open bracket", ch)
		}
	}
	for _, ch := range []byte{')', ']', '}'} {
		if isOpenBracket(ch) {
			t.Errorf("expected %c NOT to be an open bracket", ch)
		}
	}
}

// ---------------------------------------------------------------------------
// isInsideString
// ---------------------------------------------------------------------------

func TestIsInsideString_DoubleQuotes(t *testing.T) {
	line := `hello "world" end`
	if isInsideString(line, 5) {
		t.Error("position 5 (space before quote) should NOT be inside string")
	}
	if !isInsideString(line, 7) {
		t.Error("position 7 (w in 'world') should be inside string")
	}
	if isInsideString(line, 14) {
		t.Error("position 14 ('e' in 'end') should NOT be inside string")
	}
}

func TestIsInsideString_SingleQuotes(t *testing.T) {
	line := "x = 'hello' + y"
	if isInsideString(line, 2) {
		t.Error("position before quote should NOT be inside string")
	}
	if !isInsideString(line, 5) {
		t.Error("position inside single quotes should be inside string")
	}
	if isInsideString(line, 14) {
		t.Error("position after closing quote should NOT be inside string")
	}
}

func TestIsInsideString_Empty(t *testing.T) {
	if isInsideString("", 0) {
		t.Error("empty string should not contain position inside quotes")
	}
}

func TestIsInsideString_NoQuotes(t *testing.T) {
	if isInsideString("hello world", 5) {
		t.Error("line with no quotes should never return true")
	}
}

// ---------------------------------------------------------------------------
// findMatchingBracket — additional edge cases not in editor_advanced_test.go
// ---------------------------------------------------------------------------

func TestFindMatchingBracket_CloseParen(t *testing.T) {
	e := Editor{
		content: []string{"foo(bar)"},
		cursor:  0,
		col:     7, // on ')'
	}
	ln, c, ok := e.findMatchingBracket()
	if !ok {
		t.Fatal("expected match found")
	}
	if ln != 0 || c != 3 {
		t.Errorf("expected match at (0,3), got (%d,%d)", ln, c)
	}
}

func TestFindMatchingBracket_MultiLineBackward(t *testing.T) {
	e := Editor{
		content: []string{
			"func main() {",
			"    fmt.Println()",
			"}",
		},
		cursor: 2,
		col:    0, // on '}'
	}
	ln, c, ok := e.findMatchingBracket()
	if !ok {
		t.Fatal("expected match found")
	}
	if ln != 0 || c != 12 {
		t.Errorf("expected match at (0,12), got (%d,%d)", ln, c)
	}
}

func TestFindMatchingBracket_Unmatched(t *testing.T) {
	e := Editor{
		content: []string{"foo(bar"},
		cursor:  0,
		col:     3, // on '('
	}
	_, _, ok := e.findMatchingBracket()
	if ok {
		t.Error("expected no match for unmatched bracket")
	}
}

func TestFindMatchingBracket_EmptyContent(t *testing.T) {
	e := Editor{
		content: []string{},
		cursor:  0,
		col:     0,
	}
	_, _, ok := e.findMatchingBracket()
	if ok {
		t.Error("expected no match for empty content")
	}
}

func TestFindMatchingBracket_CursorAfterBracket(t *testing.T) {
	// col-1 fallback: cursor is one past bracket
	e := Editor{
		content: []string{"foo(bar)"},
		cursor:  0,
		col:     4, // one past '('
	}
	ln, c, ok := e.findMatchingBracket()
	if !ok {
		t.Fatal("expected match found via col-1 fallback")
	}
	if ln != 0 || c != 7 {
		t.Errorf("expected match at (0,7), got (%d,%d)", ln, c)
	}
}

func TestFindMatchingBracket_InsideString(t *testing.T) {
	e := Editor{
		content: []string{`x = "(" + y`},
		cursor:  0,
		col:     5, // the '(' inside quotes
	}
	_, _, ok := e.findMatchingBracket()
	if ok {
		t.Error("expected no match for bracket inside string")
	}
}

func TestFindMatchingBracket_DeeplyNested(t *testing.T) {
	e := Editor{
		content: []string{"a(b[c{d}e]f)g"},
		cursor:  0,
		col:     1, // outermost '('
	}
	ln, c, ok := e.findMatchingBracket()
	if !ok {
		t.Fatal("expected match found")
	}
	if ln != 0 || c != 11 {
		t.Errorf("expected match at (0,11) for outermost parens, got (%d,%d)", ln, c)
	}
}
