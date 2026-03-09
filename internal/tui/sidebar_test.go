package tui

import "testing"

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	if !fuzzyMatch("README", "README") {
		t.Error("fuzzyMatch(\"README\", \"README\") = false; want true")
	}
}

func TestFuzzyMatch_SubstringMatch(t *testing.T) {
	// fuzzyMatch works on character subsequence, so "read" should match
	// against the lowercased "readme.md".
	if !fuzzyMatch("readme.md", "read") {
		t.Error("fuzzyMatch(\"readme.md\", \"read\") = false; want true")
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	// The sidebar lowercases both sides before calling fuzzyMatch, so we
	// test the same way here.
	str := "readme.md"
	pattern := "readme"
	if !fuzzyMatch(str, pattern) {
		t.Errorf("fuzzyMatch(%q, %q) = false; want true", str, pattern)
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	if fuzzyMatch("readme", "xyz") {
		t.Error("fuzzyMatch(\"readme\", \"xyz\") = true; want false")
	}
}

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	// An empty pattern should match everything (all 0 pattern chars consumed).
	if !fuzzyMatch("readme.md", "") {
		t.Error("fuzzyMatch(\"readme.md\", \"\") = false; want true")
	}
	if !fuzzyMatch("", "") {
		t.Error("fuzzyMatch(\"\", \"\") = false; want true")
	}
}

func TestIsHiddenPath_DotFile(t *testing.T) {
	if !isHiddenPath(".gitignore") {
		t.Error("isHiddenPath(\".gitignore\") = false; want true")
	}
}

func TestIsHiddenPath_DotDir(t *testing.T) {
	if !isHiddenPath(".config/file") {
		t.Error("isHiddenPath(\".config/file\") = false; want true")
	}
}

func TestIsHiddenPath_Normal(t *testing.T) {
	if isHiddenPath("notes/file.md") {
		t.Error("isHiddenPath(\"notes/file.md\") = true; want false")
	}
}

func TestIsDailyNote_ValidDate(t *testing.T) {
	if !isDailyNote("2024-01-15.md") {
		t.Error("isDailyNote(\"2024-01-15.md\") = false; want true")
	}
}

func TestIsDailyNote_InvalidFormat(t *testing.T) {
	if isDailyNote("not-a-date.md") {
		t.Error("isDailyNote(\"not-a-date.md\") = true; want false")
	}
}
