package tui

import "testing"

func TestSplitTrimCSV_Basic(t *testing.T) {
	result := splitTrimCSV("go, tui, notes")
	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}
	if result[0] != "go" || result[1] != "tui" || result[2] != "notes" {
		t.Errorf("unexpected items: %v", result)
	}
}

func TestSplitTrimCSV_Empty(t *testing.T) {
	result := splitTrimCSV("")
	if len(result) != 0 {
		t.Errorf("expected 0 items for empty string, got %d", len(result))
	}
}

func TestSplitTrimCSV_Whitespace(t *testing.T) {
	result := splitTrimCSV("  ,  ,  ")
	if len(result) != 0 {
		t.Errorf("expected 0 items for whitespace-only, got %d", len(result))
	}
}

func TestFmIsDateStr_Valid(t *testing.T) {
	if !fmIsDateStr("2026-04-09") {
		t.Error("expected valid date")
	}
}

func TestFmIsDateStr_Invalid(t *testing.T) {
	for _, s := range []string{"not-a-date", "2026-13-01", "04-09-2026", "2026/04/09", ""} {
		if fmIsDateStr(s) {
			t.Errorf("expected invalid date for %q", s)
		}
	}
}

func TestFmIsNumeric_Integers(t *testing.T) {
	for _, s := range []string{"42", "0", "-5", "100"} {
		if !fmIsNumeric(s) {
			t.Errorf("expected numeric for %q", s)
		}
	}
}

func TestFmIsNumeric_Floats(t *testing.T) {
	for _, s := range []string{"3.14", "-0.5", "100.0"} {
		if !fmIsNumeric(s) {
			t.Errorf("expected numeric for %q", s)
		}
	}
}

func TestFmIsNumeric_Invalid(t *testing.T) {
	for _, s := range []string{"abc", "12a", "", "3.1.4", "1,000"} {
		if fmIsNumeric(s) {
			t.Errorf("expected non-numeric for %q", s)
		}
	}
}

func TestClassifyField_Tags(t *testing.T) {
	fe := &FrontmatterEditor{}
	f := fe.classifyField("tags", "[go, tui]")
	if f.kind != ftTags {
		t.Errorf("expected ftTags, got %d", f.kind)
	}
	if len(f.listVals) != 2 {
		t.Errorf("expected 2 list vals, got %d", len(f.listVals))
	}
}

func TestClassifyField_Bool(t *testing.T) {
	fe := &FrontmatterEditor{}
	f := fe.classifyField("published", "true")
	if f.kind != ftBool {
		t.Errorf("expected ftBool, got %d", f.kind)
	}
	if !f.boolVal {
		t.Error("expected boolVal=true")
	}
}

func TestClassifyField_Date(t *testing.T) {
	fe := &FrontmatterEditor{}
	f := fe.classifyField("date", "2026-04-09")
	if f.kind != ftDate {
		t.Errorf("expected ftDate, got %d", f.kind)
	}
}

func TestClassifyField_Number(t *testing.T) {
	fe := &FrontmatterEditor{}
	f := fe.classifyField("priority", "3")
	if f.kind != ftNumber {
		t.Errorf("expected ftNumber, got %d", f.kind)
	}
}

func TestClassifyField_String(t *testing.T) {
	fe := &FrontmatterEditor{}
	f := fe.classifyField("title", "My Note")
	if f.kind != ftString {
		t.Errorf("expected ftString, got %d", f.kind)
	}
}

func TestParseFrontmatter(t *testing.T) {
	fe := &FrontmatterEditor{}
	content := "---\ntitle: Test Note\ntags: [go, tui]\ndate: 2026-04-09\npublished: true\n---\n\n# Content"
	fe.parseFrontmatter(content)

	if len(fe.fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(fe.fields))
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	fe := &FrontmatterEditor{}
	fe.parseFrontmatter("# Just content")
	if len(fe.fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(fe.fields))
	}
}

func TestParseFrontmatter_MalformedNoClose(t *testing.T) {
	fe := &FrontmatterEditor{}
	fe.parseFrontmatter("---\ntitle: Test\nno closing")
	if len(fe.fields) != 0 {
		t.Errorf("expected 0 fields for unclosed frontmatter, got %d", len(fe.fields))
	}
}
