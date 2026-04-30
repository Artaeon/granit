package objects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestView_Validate_OK(t *testing.T) {
	v := View{
		ID: "x", Name: "X", Description: "x",
		Where: []ViewClause{{Property: "status", Op: ViewOpEq, Value: "open"}},
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestView_Validate_Errors(t *testing.T) {
	cases := []struct {
		name string
		v    View
	}{
		{"missing id", View{Name: "X", Description: "x"}},
		{"missing name", View{ID: "x", Description: "x"}},
		{"missing description", View{ID: "x", Name: "X"}},
		{"unknown op", View{ID: "x", Name: "X", Description: "x",
			Where: []ViewClause{{Property: "p", Op: "regexp", Value: ".*"}}}},
		{"empty property", View{ID: "x", Name: "X", Description: "x",
			Where: []ViewClause{{Property: "", Op: ViewOpEq, Value: "y"}}}},
		{"negative limit", View{ID: "x", Name: "X", Description: "x", Limit: -1}},
		{"bad sort dir", View{ID: "x", Name: "X", Description: "x",
			Sort: &ViewSort{Property: "title", Direction: "sideways"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.v.Validate(); err == nil {
				t.Fatalf("expected validation error, got nil")
			}
		})
	}
}

func mkObj(typeID, path, title string, props map[string]string) *Object {
	if props == nil {
		props = map[string]string{}
	}
	return &Object{TypeID: typeID, NotePath: path, Title: title, Properties: props}
}

func mkIndex(objs ...*Object) *Index {
	idx := NewIndex()
	for _, o := range objs {
		idx.byType[o.TypeID] = append(idx.byType[o.TypeID], o)
		idx.byPath[o.NotePath] = o
	}
	return idx
}

func TestEvaluate_FilterByTypeAndStatus(t *testing.T) {
	idx := mkIndex(
		mkObj("article", "a.md", "A", map[string]string{"status": "to-read"}),
		mkObj("article", "b.md", "B", map[string]string{"status": "read"}),
		mkObj("article", "c.md", "C", map[string]string{"status": "to-read"}),
		mkObj("book", "d.md", "D", map[string]string{"status": "to-read"}),
	)
	v := View{
		ID: "x", Name: "X", Description: "x",
		Type: "article",
		Where: []ViewClause{
			{Property: "status", Op: ViewOpNe, Value: "read"},
		},
	}
	got := Evaluate(idx, v)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(got), got)
	}
	titles := []string{got[0].Title, got[1].Title}
	if titles[0] != "A" || titles[1] != "C" {
		t.Fatalf("expected [A C] sorted, got %v", titles)
	}
}

func TestEvaluate_AnyType(t *testing.T) {
	idx := mkIndex(
		mkObj("article", "a.md", "A", nil),
		mkObj("book", "b.md", "B", nil),
	)
	v := View{ID: "x", Name: "X", Description: "x"}
	got := Evaluate(idx, v)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
}

func TestEvaluate_OperatorMatrix(t *testing.T) {
	objs := []*Object{
		mkObj("t", "a", "A", map[string]string{"k": "Apple", "n": "5"}),
		mkObj("t", "b", "B", map[string]string{"k": "Banana", "n": "3"}),
		mkObj("t", "c", "C", map[string]string{}), // missing
	}
	idx := mkIndex(objs...)

	cases := []struct {
		op       ViewOp
		prop, v  string
		wantSize int
	}{
		{ViewOpEq, "k", "apple", 1},   // case-insensitive
		{ViewOpNe, "k", "Apple", 2},
		{ViewOpContains, "k", "an", 1}, // banana
		{ViewOpExists, "k", "", 2},
		{ViewOpMissing, "k", "", 1},
		{ViewOpGt, "n", "4", 1},
		{ViewOpLt, "n", "4", 1},
	}
	for _, tc := range cases {
		t.Run(string(tc.op), func(t *testing.T) {
			v := View{ID: "x", Name: "X", Description: "x",
				Where: []ViewClause{{Property: tc.prop, Op: tc.op, Value: tc.v}}}
			got := Evaluate(idx, v)
			if len(got) != tc.wantSize {
				t.Fatalf("op %s: expected %d, got %d (%+v)", tc.op, tc.wantSize, len(got), got)
			}
		})
	}
}

func TestEvaluate_SortNumericDesc(t *testing.T) {
	idx := mkIndex(
		mkObj("t", "a", "A", map[string]string{"rating": "3"}),
		mkObj("t", "b", "B", map[string]string{"rating": "5"}),
		mkObj("t", "c", "C", map[string]string{"rating": "1"}),
	)
	v := View{ID: "x", Name: "X", Description: "x",
		Sort: &ViewSort{Property: "rating", Direction: "desc"}}
	got := Evaluate(idx, v)
	if len(got) != 3 || got[0].Title != "B" || got[2].Title != "C" {
		t.Fatalf("expected B,A,C; got %v", titles(got))
	}
}

func TestEvaluate_LimitTrims(t *testing.T) {
	idx := mkIndex(
		mkObj("t", "a", "A", nil),
		mkObj("t", "b", "B", nil),
		mkObj("t", "c", "C", nil),
	)
	v := View{ID: "x", Name: "X", Description: "x", Limit: 2}
	got := Evaluate(idx, v)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestEvaluate_PromotedFieldsTitleAndType(t *testing.T) {
	idx := mkIndex(
		mkObj("article", "a.md", "Apple", nil),
		mkObj("book", "b.md", "Banana", nil),
	)
	// where title contains 'pp'
	v := View{ID: "x", Name: "X", Description: "x",
		Where: []ViewClause{{Property: "title", Op: ViewOpContains, Value: "pp"}}}
	got := Evaluate(idx, v)
	if len(got) != 1 || got[0].Title != "Apple" {
		t.Fatalf("expected only Apple, got %v", titles(got))
	}
}

func TestEvaluate_GtBestEffortBadNumberExcludes(t *testing.T) {
	idx := mkIndex(
		mkObj("t", "a", "A", map[string]string{"n": "five"}),
		mkObj("t", "b", "B", map[string]string{"n": "5"}),
	)
	v := View{ID: "x", Name: "X", Description: "x",
		Where: []ViewClause{{Property: "n", Op: ViewOpGt, Value: "3"}}}
	got := Evaluate(idx, v)
	if len(got) != 1 || got[0].Title != "B" {
		t.Fatalf("expected only B (numeric); got %v", titles(got))
	}
}

func TestViewCatalog_BuiltinAndOverride(t *testing.T) {
	tmp := t.TempDir()
	// Override "articles-to-read" with a different definition.
	overrideDir := filepath.Join(tmp, ".granit", "views")
	if err := os.MkdirAll(overrideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := View{
		ID: "articles-to-read", Name: "Custom Articles",
		Description: "vault override", Type: "article",
	}
	data, _ := json.Marshal(override)
	if err := os.WriteFile(filepath.Join(overrideDir, "articles-to-read.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cat := NewViewCatalog(BuiltinViews())
	loaded, errs := cat.LoadVaultDir(tmp)
	if loaded != 1 || len(errs) != 0 {
		t.Fatalf("expected 1 loaded, 0 errs; got %d / %v", loaded, errs)
	}
	got, ok := cat.ByID("articles-to-read")
	if !ok || got.Name != "Custom Articles" {
		t.Fatalf("override didn't apply: %+v", got)
	}
	// Built-ins still present
	if _, ok := cat.ByID("recent-highlights"); !ok {
		t.Fatalf("built-in dropped")
	}
}

func TestViewCatalog_FilenameMustMatchID(t *testing.T) {
	tmp := t.TempDir()
	overrideDir := filepath.Join(tmp, ".granit", "views")
	_ = os.MkdirAll(overrideDir, 0o755)
	v := View{ID: "wrong-id", Name: "X", Description: "x"}
	data, _ := json.Marshal(v)
	_ = os.WriteFile(filepath.Join(overrideDir, "different-name.json"), data, 0o644)

	cat := NewViewCatalog(nil)
	loaded, errs := cat.LoadVaultDir(tmp)
	if loaded != 0 || len(errs) != 1 {
		t.Fatalf("expected 0/1, got %d/%d", loaded, len(errs))
	}
}

func TestSaveView_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	v := View{ID: "round", Name: "R", Description: "r",
		Type: "article", Limit: 10}
	if err := SaveView(tmp, v); err != nil {
		t.Fatal(err)
	}
	cat := NewViewCatalog(nil)
	loaded, errs := cat.LoadVaultDir(tmp)
	if loaded != 1 || len(errs) != 0 {
		t.Fatalf("expected 1/0, got %d/%v", loaded, errs)
	}
	got, _ := cat.ByID("round")
	if got.Name != "R" || got.Limit != 10 {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestBuiltinViews_AllValid(t *testing.T) {
	for _, v := range BuiltinViews() {
		if err := v.Validate(); err != nil {
			t.Errorf("built-in %q invalid: %v", v.ID, err)
		}
	}
}

func titles(objs []*Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.Title
	}
	return out
}
