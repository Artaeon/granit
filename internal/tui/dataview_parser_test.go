package tui

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Valid TABLE query with fields, FROM, WHERE, SORT, LIMIT
// ---------------------------------------------------------------------------

func TestParseDVQuery_FullTableQuery(t *testing.T) {
	q := ParseDVQuery(`TABLE status, priority FROM "projects" WHERE status = "active" SORT priority DESC LIMIT 10`)

	if q.Mode != DVModeTable {
		t.Errorf("expected DVModeTable, got %d", q.Mode)
	}
	if len(q.Fields) != 2 || q.Fields[0] != "status" || q.Fields[1] != "priority" {
		t.Errorf("expected fields [status, priority], got %v", q.Fields)
	}
	if q.Source != "projects" {
		t.Errorf("expected source 'projects', got %q", q.Source)
	}
	if len(q.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Conditions))
	}
	c := q.Conditions[0]
	if c.Field != "status" || c.Op != "=" || c.Value != "active" {
		t.Errorf("expected condition status = active, got %s %s %s", c.Field, c.Op, c.Value)
	}
	if q.Sort == nil || q.Sort.Field != "priority" || !q.Sort.Desc {
		t.Errorf("expected SORT priority DESC, got %+v", q.Sort)
	}
	if q.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Limit)
	}
}

// ---------------------------------------------------------------------------
// Valid LIST query
// ---------------------------------------------------------------------------

func TestParseDVQuery_ListQuery(t *testing.T) {
	q := ParseDVQuery(`LIST FROM #journal WHERE date >= 2024-01-01 SORT date DESC`)

	if q.Mode != DVModeList {
		t.Errorf("expected DVModeList, got %d", q.Mode)
	}
	if q.SourceTag != "journal" {
		t.Errorf("expected source tag 'journal', got %q", q.SourceTag)
	}
	if len(q.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Conditions))
	}
	c := q.Conditions[0]
	if c.Field != "date" || c.Op != ">=" || c.Value != "2024-01-01" {
		t.Errorf("expected date >= 2024-01-01, got %s %s %s", c.Field, c.Op, c.Value)
	}
	if q.Sort == nil || q.Sort.Field != "date" || !q.Sort.Desc {
		t.Errorf("expected SORT date DESC, got %+v", q.Sort)
	}
}

// ---------------------------------------------------------------------------
// Valid TASK query
// ---------------------------------------------------------------------------

func TestParseDVQuery_TaskQuery(t *testing.T) {
	q := ParseDVQuery(`TASK FROM "projects" WHERE !completed`)

	if q.Mode != DVModeTask {
		t.Errorf("expected DVModeTask, got %d", q.Mode)
	}
	if q.Source != "projects" {
		t.Errorf("expected source 'projects', got %q", q.Source)
	}
	if len(q.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Conditions))
	}
	c := q.Conditions[0]
	if c.Field != "completed" || !c.Negate {
		t.Errorf("expected negated 'completed' condition, got field=%q negate=%v", c.Field, c.Negate)
	}
}

// ---------------------------------------------------------------------------
// Empty query returns default
// ---------------------------------------------------------------------------

func TestParseDVQuery_EmptyQuery(t *testing.T) {
	q := ParseDVQuery("")

	if q.Mode != DVModeTable {
		t.Errorf("expected default DVModeTable, got %d", q.Mode)
	}
	if len(q.Fields) != 0 {
		t.Errorf("expected no fields, got %v", q.Fields)
	}
	if q.Source != "" {
		t.Errorf("expected empty source, got %q", q.Source)
	}
	if len(q.Conditions) != 0 {
		t.Errorf("expected no conditions, got %d", len(q.Conditions))
	}
	if q.Sort != nil {
		t.Errorf("expected nil sort, got %+v", q.Sort)
	}
	if q.Limit != 0 {
		t.Errorf("expected limit 0, got %d", q.Limit)
	}
}

// ---------------------------------------------------------------------------
// Malformed query: unclosed quotes
// ---------------------------------------------------------------------------

func TestParseDVQuery_UnclosedQuotes(t *testing.T) {
	// Should not panic; tokenizer consumes up to end of string
	q := ParseDVQuery(`TABLE title FROM "unclosed`)

	if q.Mode != DVModeTable {
		t.Errorf("expected DVModeTable, got %d", q.Mode)
	}
	// The unclosed quote still produces a token with the content after the quote
	if q.Source != "unclosed" {
		t.Errorf("expected source 'unclosed', got %q", q.Source)
	}
}

// ---------------------------------------------------------------------------
// Unknown operator silently handled
// ---------------------------------------------------------------------------

func TestParseDVQuery_UnknownOperator(t *testing.T) {
	// "LIKE" is not a supported operator; the parser should skip the condition
	q := ParseDVQuery(`TABLE title WHERE name LIKE "test"`)

	// The unknown operator causes the condition to be skipped via continue,
	// so no condition should be recorded for that triple
	for _, c := range q.Conditions {
		if c.Op == "LIKE" {
			t.Errorf("unknown operator LIKE should not appear in conditions")
		}
	}
}

// ---------------------------------------------------------------------------
// LIMIT with invalid values
// ---------------------------------------------------------------------------

func TestParseDVQuery_LimitNegative(t *testing.T) {
	q := ParseDVQuery(`TABLE title LIMIT -5`)
	if q.Limit != 0 {
		t.Errorf("negative LIMIT should be treated as 0, got %d", q.Limit)
	}
}

func TestParseDVQuery_LimitZero(t *testing.T) {
	q := ParseDVQuery(`TABLE title LIMIT 0`)
	if q.Limit != 0 {
		t.Errorf("zero LIMIT should remain 0, got %d", q.Limit)
	}
}

func TestParseDVQuery_LimitNonNumeric(t *testing.T) {
	q := ParseDVQuery(`TABLE title LIMIT abc`)
	if q.Limit != 0 {
		t.Errorf("non-numeric LIMIT should be treated as 0, got %d", q.Limit)
	}
}

// ---------------------------------------------------------------------------
// Query with only keywords: "TABLE FROM"
// ---------------------------------------------------------------------------

func TestParseDVQuery_TableFromNoSource(t *testing.T) {
	// "TABLE FROM" — no fields and FROM has no source token after it
	// because there are no more tokens. The parser should still return
	// a valid struct without panicking.
	q := ParseDVQuery(`TABLE FROM`)

	if q.Mode != DVModeTable {
		t.Errorf("expected DVModeTable, got %d", q.Mode)
	}
	// "FROM" is a keyword so parseFieldList stops before it,
	// but there's no token after FROM for the source
	if len(q.Fields) != 0 {
		t.Errorf("expected no fields, got %v", q.Fields)
	}
}

// ---------------------------------------------------------------------------
// Unicode in field values
// ---------------------------------------------------------------------------

func TestParseDVQuery_UnicodeFieldValues(t *testing.T) {
	q := ParseDVQuery(`TABLE title WHERE author = "Muller"`)

	if len(q.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Conditions))
	}
	if q.Conditions[0].Value != "Muller" {
		t.Errorf("expected value 'Muller', got %q", q.Conditions[0].Value)
	}

	// Test with actual unicode characters
	q2 := ParseDVQuery(`TABLE title WHERE category = "日本語"`)
	if len(q2.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q2.Conditions))
	}
	if q2.Conditions[0].Value != "日本語" {
		t.Errorf("expected value '日本語', got %q", q2.Conditions[0].Value)
	}
}

// ---------------------------------------------------------------------------
// tokenizeDV edge cases
// ---------------------------------------------------------------------------

func TestTokenizeDV_ComparisonOperators(t *testing.T) {
	tokens := tokenizeDV("a >= b")
	if len(tokens) != 3 || tokens[1] != ">=" {
		t.Errorf("expected [a >= b], got %v", tokens)
	}

	tokens = tokenizeDV("x != y")
	if len(tokens) != 3 || tokens[1] != "!=" {
		t.Errorf("expected [x != y], got %v", tokens)
	}

	tokens = tokenizeDV("x <= y")
	if len(tokens) != 3 || tokens[1] != "<=" {
		t.Errorf("expected [x <= y], got %v", tokens)
	}
}

func TestTokenizeDV_CommaSeparatedFields(t *testing.T) {
	tokens := tokenizeDV("a, b, c")
	// Commas are consumed as separators
	if len(tokens) != 3 || tokens[0] != "a" || tokens[1] != "b" || tokens[2] != "c" {
		t.Errorf("expected [a b c], got %v", tokens)
	}
}

// ---------------------------------------------------------------------------
// SORT ASC (explicit ascending)
// ---------------------------------------------------------------------------

func TestParseDVQuery_SortASC(t *testing.T) {
	q := ParseDVQuery(`TABLE title SORT date ASC`)
	if q.Sort == nil {
		t.Fatal("expected sort clause, got nil")
	}
	if q.Sort.Field != "date" {
		t.Errorf("expected sort field 'date', got %q", q.Sort.Field)
	}
	if q.Sort.Desc {
		t.Error("expected ASC (Desc=false), got DESC")
	}
}

// ---------------------------------------------------------------------------
// Multiple WHERE conditions with AND
// ---------------------------------------------------------------------------

func TestParseDVQuery_MultipleConditions(t *testing.T) {
	q := ParseDVQuery(`TABLE title WHERE status = "active" AND priority = "high"`)

	if len(q.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(q.Conditions))
	}
	if q.Conditions[0].Field != "status" || q.Conditions[0].Value != "active" {
		t.Errorf("condition 0: expected status=active, got %s=%s", q.Conditions[0].Field, q.Conditions[0].Value)
	}
	if q.Conditions[1].Field != "priority" || q.Conditions[1].Value != "high" {
		t.Errorf("condition 1: expected priority=high, got %s=%s", q.Conditions[1].Field, q.Conditions[1].Value)
	}
}

// ---------------------------------------------------------------------------
// CONTAINS operator
// ---------------------------------------------------------------------------

func TestParseDVQuery_ContainsOperator(t *testing.T) {
	q := ParseDVQuery(`TABLE title WHERE tags CONTAINS "go"`)

	if len(q.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(q.Conditions))
	}
	c := q.Conditions[0]
	if c.Field != "tags" || c.Op != "CONTAINS" || c.Value != "go" {
		t.Errorf("expected tags CONTAINS go, got %s %s %s", c.Field, c.Op, c.Value)
	}
}

// ---------------------------------------------------------------------------
// RawQuery is preserved
// ---------------------------------------------------------------------------

func TestParseDVQuery_RawQueryPreserved(t *testing.T) {
	raw := `TABLE title FROM "notes" LIMIT 5`
	q := ParseDVQuery(raw)
	if q.RawQuery != raw {
		t.Errorf("expected RawQuery %q, got %q", raw, q.RawQuery)
	}
}
