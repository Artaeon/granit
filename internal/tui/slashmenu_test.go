package tui

import "testing"

func TestSlashMenu_ActivateModeAllShowsBothInsertAndAI(t *testing.T) {
	sm := NewSlashMenu()
	sm.Activate(0, 0)
	if sm.Mode() != SlashMenuModeAll {
		t.Fatalf("expected modeAll, got %v", sm.Mode())
	}
	hasInsert, hasAI := false, false
	for _, it := range sm.matches {
		if it.Action != "" {
			hasAI = true
		}
		if it.Insert != "" && it.Action == "" {
			hasInsert = true
		}
	}
	if !hasInsert || !hasAI {
		t.Fatalf("modeAll should expose both insert + AI items (insert=%v ai=%v)", hasInsert, hasAI)
	}
}

func TestSlashMenu_ActivateAIShowsOnlyAIItems(t *testing.T) {
	sm := NewSlashMenu()
	sm.ActivateAI(0, 0)
	if sm.Mode() != SlashMenuModeAI {
		t.Fatalf("expected modeAI, got %v", sm.Mode())
	}
	if len(sm.matches) == 0 {
		t.Fatal("modeAI yielded no items")
	}
	for _, it := range sm.matches {
		if it.Action == "" {
			t.Fatalf("modeAI should only contain AI items; got insert item %q", it.Label)
		}
	}
}

func TestSlashMenu_HandleEnterReturnsItemPointer(t *testing.T) {
	sm := NewSlashMenu()
	sm.ActivateAI(0, 0)
	// Cursor is at item 0 — first AI action.
	item, consumed, closed := sm.HandleKey("enter")
	if item == nil {
		t.Fatal("expected item back from Enter")
	}
	if !consumed || !closed {
		t.Fatalf("Enter should be consumed and close the menu; got consumed=%v closed=%v", consumed, closed)
	}
	if item.Action == "" {
		t.Fatalf("expected an AI action; got %+v", item)
	}
	if sm.IsActive() {
		t.Fatal("menu should be closed after Enter")
	}
}

func TestSlashMenu_HandleEscClosesWithoutSelection(t *testing.T) {
	sm := NewSlashMenu()
	sm.Activate(0, 0)
	item, consumed, closed := sm.HandleKey("esc")
	if item != nil {
		t.Fatalf("Esc should not select; got %+v", item)
	}
	if !consumed || !closed {
		t.Fatalf("Esc should be consumed and close; got consumed=%v closed=%v", consumed, closed)
	}
}

func TestSlashMenu_FilterShrinksMatches(t *testing.T) {
	sm := NewSlashMenu()
	sm.Activate(0, 0)
	full := len(sm.matches)
	// Filter to "rew" — should narrow to AI rewrite + maybe nothing else.
	if _, _, _ = sm.HandleKey("r"); !sm.IsActive() {
		t.Fatal("menu should remain active after typing")
	}
	if _, _, _ = sm.HandleKey("e"); !sm.IsActive() {
		t.Fatal("menu should remain active after typing")
	}
	if _, _, _ = sm.HandleKey("w"); !sm.IsActive() {
		t.Fatal("menu should remain active after typing")
	}
	if len(sm.matches) >= full {
		t.Fatalf("filter should have shrunk matches (full=%d, filtered=%d)", full, len(sm.matches))
	}
	// At least one "rewrite" match expected.
	found := false
	for _, it := range sm.matches {
		if it.Command == "rewrite" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find 'rewrite' in filtered matches")
	}
}
