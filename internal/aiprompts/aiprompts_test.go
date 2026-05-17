package aiprompts

import (
	"testing"
)

func TestSave_Validation(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		lib     Library
		wantErr bool
	}{
		{
			name: "empty library saves",
			lib:  Library{Entries: []Entry{}},
		},
		{
			name: "missing label rejected",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "  ", Prompt: "do x", Scope: ScopeEither},
			}},
			wantErr: true,
		},
		{
			name: "missing prompt rejected",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "rewrite", Prompt: "", Scope: ScopeEither},
			}},
			wantErr: true,
		},
		{
			name: "invalid scope rejected",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "x", Prompt: "do x", Scope: "bogus"},
			}},
			wantErr: true,
		},
		{
			name: "empty scope defaults to either",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "x", Prompt: "do x"},
			}},
		},
		{
			name: "duplicate id rejected",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "x", Prompt: "px", Scope: ScopeEither},
				{ID: "a", Label: "y", Prompt: "py", Scope: ScopeEither},
			}},
			wantErr: true,
		},
		{
			name: "valid library saves",
			lib: Library{Entries: []Entry{
				{ID: "a", Label: "tighten", Prompt: "Make this tighter without losing meaning.", Scope: ScopeSelection},
				{ID: "b", Label: "outline", Prompt: "Produce an H2/H3 outline.", Scope: ScopeCursor},
			}},
		},
	}
	for _, c := range cases {
		err := Save(dir, c.lib)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Library{Entries: []Entry{
		{ID: "n1", Label: "voice", Prompt: "Rewrite in my voice.", Scope: ScopeSelection},
		{ID: "n2", Label: "brainstorm", Prompt: "Brainstorm 5 angles.", Scope: ScopeCursor},
	}}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	out := Load(dir)
	if len(out.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(out.Entries))
	}
	if out.Entries[0].CreatedAt == "" {
		t.Error("Save should stamp CreatedAt on new entries")
	}
	if out.UpdatedAt == "" {
		t.Error("Save should stamp UpdatedAt")
	}
}

func TestLoad_Missing(t *testing.T) {
	dir := t.TempDir()
	out := Load(dir)
	if out.Entries == nil {
		t.Fatal("Load on missing file should return empty (not nil) entries slice")
	}
	if len(out.Entries) != 0 {
		t.Fatalf("got %d entries on empty load, want 0", len(out.Entries))
	}
}
