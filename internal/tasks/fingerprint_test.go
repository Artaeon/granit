package tasks

import "testing"

func TestNormalize_StripsCheckboxPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"- [ ] buy milk", "buy milk"},
		{"  - [x] done thing", "done thing"},
		{"* [ ] alt bullet", "alt bullet"},
		{"+ [X] capital x", "capital x"},
		{"\t- [ ] tab indent", "tab indent"},
	}
	for _, c := range cases {
		if got := NormalizeTaskText(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalize_StripsTrailingMetadata(t *testing.T) {
	cases := []struct{ in, want string }{
		// Each metadata chunk individually
		{"- [ ] write doc 📅 2026-04-30", "write doc"},
		{"- [ ] write doc 🛫 2026-04-25", "write doc"},
		{"- [ ] write doc ⏳ 2026-04-26", "write doc"},
		{"- [ ] write doc ✅ 2026-04-25", "write doc"},
		{"- [ ] write doc ⏰ 14:00-15:30", "write doc"},
		{"- [ ] write doc ⏰ 9:00", "write doc"},
		{"- [ ] write doc 🔺", "write doc"},
		{"- [ ] write doc ⏫", "write doc"},
		{"- [ ] write doc 🔼", "write doc"},
		{"- [ ] write doc 🔽", "write doc"},
		{"- [ ] write doc ⏬", "write doc"},
		{"- [ ] write doc p:3", "write doc"},
		{"- [ ] write doc ~30m", "write doc"},
		{"- [ ] write doc 🔁 daily", "write doc"},
		{"- [ ] write doc [note:reviewed]", "write doc"},
		{"- [ ] write doc goal:G004", "write doc"},
		{"- [ ] write doc snooze:2026-05-01", "write doc"},

		// Combined
		{"- [ ] ship phase 2 📅 2026-04-30 ⏫ ~90m goal:G001", "ship phase 2"},
	}
	for _, c := range cases {
		if got := NormalizeTaskText(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalize_PreservesTags(t *testing.T) {
	// Tags are deliberately load-bearing — moving #shopping to
	// #personal is a meaningful enough edit that we'd rather hit
	// fuzzy matching than silently re-attribute.
	got := NormalizeTaskText("- [ ] buy milk #shopping #urgent")
	want := "buy milk #shopping #urgent"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNormalize_CollapsesWhitespace(t *testing.T) {
	got := NormalizeTaskText("- [ ]   too   much    space   ")
	want := "too much space"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestNormalize_LowercasesText(t *testing.T) {
	got := NormalizeTaskText("- [ ] Ship Phase 2 Design Doc")
	want := "ship phase 2 design doc"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestFingerprint_StableAcrossEquivalentEdits(t *testing.T) {
	// Toggling done, adding/removing trailing metadata, and indent
	// changes must produce the same fingerprint so the reconciler
	// stays glued to the same sidecar entry.
	base := "- [ ] ship phase 2 design doc"
	equivalents := []string{
		"- [x] ship phase 2 design doc",                       // done toggled
		"  - [ ] ship phase 2 design doc",                     // indented
		"- [ ] ship phase 2 design doc 📅 2026-04-30",         // due added
		"- [ ] ship phase 2 design doc ⏫ ~90m goal:G001",     // priority+estimate+goal added
		"- [ ] Ship Phase 2 Design Doc",                       // case differs
		"- [ ]   ship  phase  2  design  doc  ",               // whitespace differs
	}
	want := Fingerprint(base)
	for _, e := range equivalents {
		if got := Fingerprint(e); got != want {
			t.Errorf("Fingerprint(%q)=%q != Fingerprint(%q)=%q", e, got, base, want)
		}
	}
}

func TestFingerprint_DifferentForDifferentText(t *testing.T) {
	a := Fingerprint("- [ ] ship phase 2")
	b := Fingerprint("- [ ] ship phase 3")
	if a == b {
		t.Errorf("expected distinct fingerprints, both = %q", a)
	}
}

func TestFingerprint_DifferentWhenTagChanges(t *testing.T) {
	// Tag changes are intentionally identity-affecting.
	a := Fingerprint("- [ ] buy milk #shopping")
	b := Fingerprint("- [ ] buy milk #personal")
	if a == b {
		t.Errorf("tag change should change fingerprint, both = %q", a)
	}
}
