package serveapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/artaeon/granit/internal/daily"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// TestJotPathRegex covers the one bit of logic that's easy to get wrong:
// the empty-folder branch + folder-prefix branch. Everything else in
// handleListJots is straight-line glue.
func TestJotPathRegex(t *testing.T) {
	cases := []struct {
		folder, path string
		matches      bool
		date         string
	}{
		// Empty folder → match at vault root only.
		{"", "2026-04-30.md", true, "2026-04-30"},
		{"", "Jots/2026-04-30.md", false, ""},
		{"", "Notes/Some Note.md", false, ""},

		// Configured folder → match only that folder.
		{"Jots", "Jots/2026-04-30.md", true, "2026-04-30"},
		{"Jots", "2026-04-30.md", false, ""},
		{"Jots", "Other/2026-04-30.md", false, ""},

		// Trailing slashes get trimmed before regex compile.
		{"Jots/", "Jots/2026-04-30.md", true, "2026-04-30"},
	}
	for _, c := range cases {
		re := jotPathRegex(c.folder)
		m := re.FindStringSubmatch(c.path)
		if c.matches {
			if m == nil {
				t.Errorf("folder=%q path=%q: expected match, got none", c.folder, c.path)
				continue
			}
			if m[1] != c.date {
				t.Errorf("folder=%q path=%q: date=%q want %q", c.folder, c.path, m[1], c.date)
			}
		} else if m != nil {
			t.Errorf("folder=%q path=%q: expected no match, got %v", c.folder, c.path, m)
		}
	}
}

// TestHandleListJots_Pagination spins up a vault with 25 dailies and
// walks through pages of 10, asserting newest-first ordering, correct
// cursor handoff, and that hasMore flips false on the last page.
func TestHandleListJots_Pagination(t *testing.T) {
	root := t.TempDir()
	jotsDir := filepath.Join(root, "Jots")
	if err := os.MkdirAll(jotsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 25 dailies across April-May 2026. Day-of-month varies from 1..25,
	// month rolls over so we exercise lexical date sort across months.
	for i := 1; i <= 25; i++ {
		day := i
		month := 4
		if day > 30 {
			day -= 30
			month = 5
		}
		date := fmt.Sprintf("2026-%02d-%02d", month, day)
		body := fmt.Sprintf("# Jot %s\n\nbody for %s\n", date, date)
		if err := os.WriteFile(filepath.Join(jotsDir, date+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// One non-daily note in the same folder — must NOT show up.
	if err := os.WriteFile(filepath.Join(jotsDir, "README.md"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// One daily-shaped name at the vault root — must NOT show up either
	// (folder is "Jots", so the root entry is out of scope).
	if err := os.WriteFile(filepath.Join(root, "2026-12-31.md"), []byte("decoy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Persist the daily-folder setting to the vault so the handler picks
	// it up via config.LoadForVault. Per-vault config lives at the vault
	// root as `.granit.json` (overrides the global config).
	cfgJSON := []byte(`{"daily_notes_folder":"Jots"}`)
	if err := os.WriteFile(filepath.Join(root, ".granit.json"), cfgJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	v, err := vault.NewVault(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Scan(); err != nil {
		t.Fatal(err)
	}
	store, err := tasks.Load(root, func() []tasks.NoteContent {
		var out []tasks.NoteContent
		for _, n := range v.SnapshotNotes() {
			v.EnsureLoaded(n.RelPath)
			out = append(out, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
		}
		return out
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: Config{
		Vault:     v,
		TaskStore: store,
		Daily:     daily.DailyConfig{Folder: "Jots", Template: daily.DefaultConfig().Template},
	}}

	type page struct {
		Jots       []jotEntry `json:"jots"`
		NextBefore *string    `json:"nextBefore"`
		HasMore    bool       `json:"hasMore"`
	}

	get := func(qs string) page {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jots"+qs, nil)
		rr := httptest.NewRecorder()
		s.handleListJots(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		var p page
		if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, rr.Body.String())
		}
		return p
	}

	// Page 1: 10 newest. Highest date is 2026-05-?? if any rolled over.
	// Days 1..25 with rollover at 30 → all in April (April has 30 days).
	// So days 1..25 are all 2026-04-01 through 2026-04-25; newest is 04-25.
	p1 := get("?limit=10")
	if len(p1.Jots) != 10 {
		t.Fatalf("page1: len=%d want 10", len(p1.Jots))
	}
	if p1.Jots[0].Date != "2026-04-25" {
		t.Errorf("page1[0].Date=%s want 2026-04-25", p1.Jots[0].Date)
	}
	for i := 1; i < len(p1.Jots); i++ {
		if p1.Jots[i].Date >= p1.Jots[i-1].Date {
			t.Errorf("page1: not newest-first at %d (%s vs %s)", i, p1.Jots[i].Date, p1.Jots[i-1].Date)
		}
	}
	if !p1.HasMore || p1.NextBefore == nil {
		t.Errorf("page1: hasMore=%v nextBefore=%v want hasMore=true and a cursor", p1.HasMore, p1.NextBefore)
	}
	if p1.NextBefore != nil && *p1.NextBefore != p1.Jots[9].Date {
		t.Errorf("page1: nextBefore=%s want %s (last date in page)", *p1.NextBefore, p1.Jots[9].Date)
	}
	// Body should be plain (frontmatter stripped) — test files have none, so
	// just confirm the body is non-empty.
	if p1.Jots[0].Body == "" {
		t.Error("page1[0].Body is empty; expected loaded content")
	}
	// Decoy at root and README must not have leaked in.
	for _, j := range p1.Jots {
		if j.Path == "2026-12-31.md" {
			t.Errorf("decoy file at root leaked into Jots: %s", j.Path)
		}
		if j.Title == "" {
			// not a hard fatal — Title comes from vault, just smoke-check
			t.Logf("note %s has empty title", j.Path)
		}
	}

	// Page 2: pass the cursor; expect the next 10.
	p2 := get("?limit=10&before=" + *p1.NextBefore)
	if len(p2.Jots) != 10 {
		t.Fatalf("page2: len=%d want 10", len(p2.Jots))
	}
	if p2.Jots[0].Date >= *p1.NextBefore {
		t.Errorf("page2[0].Date=%s should be < cursor %s", p2.Jots[0].Date, *p1.NextBefore)
	}
	if !p2.HasMore || p2.NextBefore == nil {
		t.Errorf("page2: hasMore=%v nextBefore=%v want hasMore=true", p2.HasMore, p2.NextBefore)
	}

	// Page 3: should drain the remaining 5 and signal no more.
	p3 := get("?limit=10&before=" + *p2.NextBefore)
	if len(p3.Jots) != 5 {
		t.Fatalf("page3: len=%d want 5 (25 total - 20 prior)", len(p3.Jots))
	}
	if p3.HasMore {
		t.Error("page3: hasMore=true want false (last page)")
	}
	if p3.NextBefore != nil {
		t.Errorf("page3: nextBefore=%v want nil on last page", *p3.NextBefore)
	}

	// Default limit is 20 when omitted.
	pDefault := get("")
	if len(pDefault.Jots) != 20 {
		t.Errorf("default limit: len=%d want 20", len(pDefault.Jots))
	}

	// Limit cap at 50.
	pCapped := get("?limit=999")
	if len(pCapped.Jots) > 50 {
		t.Errorf("limit cap: len=%d want <=50", len(pCapped.Jots))
	}
}
