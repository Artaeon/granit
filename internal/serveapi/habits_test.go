package serveapi

import "testing"

func TestStripTaskMeta(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"daily blog writing / journaling", "daily blog writing / journaling"},
		{"daily blog writing / journaling ⏰ 11:30-12:00", "daily blog writing / journaling"},
		{"gym !2 due:2026-04-15", "gym"},
		{"read 20 pages #habit", "read 20 pages"},
		{"focus 🔺 #priority", "focus"},
		{"morning prayer 📅 2026-04-01", "morning prayer"},
		{"  trim me  ", "trim me"},
	}
	for _, c := range cases {
		got := stripTaskMeta(c.in)
		if got != c.want {
			t.Errorf("stripTaskMeta(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseHabitsSection(t *testing.T) {
	content := `# Daily

## Tasks

- [x] some unrelated task

## Habits

- [x] gym
- [ ] read 20 pages
- [x] daily praying ⏰ 13:45-14:15
- [ ] not a checkbox just text

## Notes

- [ ] this should NOT count (different section)
`
	got := parseHabitsSection(content)
	want := map[string]bool{
		"gym":            true,
		"read 20 pages":  false,
		"daily praying":  true,
		"not a checkbox just text": false,
	}
	if len(got) != len(want) {
		t.Errorf("parseHabitsSection: got %d items, want %d\ngot: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("parseHabitsSection[%q]: got=%v ok=%v, want %v", k, g, ok, v)
		}
	}
	if _, ok := got["this should NOT count (different section)"]; ok {
		t.Error("habit parser leaked content from outside ## Habits section")
	}
}

func TestParseHabitsSection_EmptyOrMissing(t *testing.T) {
	if got := parseHabitsSection(""); len(got) != 0 {
		t.Errorf("empty content: got %d", len(got))
	}
	if got := parseHabitsSection("# Title\n\nno habits section here\n"); len(got) != 0 {
		t.Errorf("no habits section: got %d", len(got))
	}
}

func TestComputeStreaks(t *testing.T) {
	// 5 days: done done done not done — current streak 1 (today done), longest 3.
	days := []habitDay{
		{Date: "2026-01-01", Done: true},
		{Date: "2026-01-02", Done: true},
		{Date: "2026-01-03", Done: true},
		{Date: "2026-01-04", Done: false},
		{Date: "2026-01-05", Done: true},
	}
	curr, longest := computeStreaks(days)
	if curr != 1 {
		t.Errorf("current streak: got %d, want 1", curr)
	}
	if longest != 3 {
		t.Errorf("longest streak: got %d, want 3", longest)
	}

	// All done
	all := []habitDay{{"a", true}, {"b", true}, {"c", true}}
	curr2, long2 := computeStreaks(all)
	if curr2 != 3 || long2 != 3 {
		t.Errorf("all-done streaks: got %d/%d, want 3/3", curr2, long2)
	}

	// None done
	none := []habitDay{{"a", false}, {"b", false}}
	c3, l3 := computeStreaks(none)
	if c3 != 0 || l3 != 0 {
		t.Errorf("none-done streaks: got %d/%d, want 0/0", c3, l3)
	}

	// Today undone, but yesterday and earlier done — current streak counts back from yesterday
	missed := []habitDay{
		{"2026-01-01", true},
		{"2026-01-02", true},
		{"2026-01-03", false}, // today
	}
	curr4, _ := computeStreaks(missed)
	if curr4 != 2 {
		t.Errorf("missed-today streak: got %d, want 2 (counts back from yesterday)", curr4)
	}
}

func TestPctDone(t *testing.T) {
	days := []habitDay{
		{"a", true}, {"b", true}, {"c", false}, {"d", true},
	}
	if got := pctDone(days, 4); got != 75 {
		t.Errorf("4 days, 3 done: got %d%%, want 75%%", got)
	}
	if got := pctDone(days, 2); got != 50 {
		t.Errorf("last 2 days (false, true): got %d%%, want 50%%", got)
	}
	if got := pctDone(days, 100); got != 75 {
		t.Errorf("more days than have: got %d%%, want 75%% (clamped)", got)
	}
	if got := pctDone(nil, 10); got != 0 {
		t.Errorf("empty: got %d%%, want 0%%", got)
	}
}

func TestDailyDate(t *testing.T) {
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{"2026-04-30.md", "2026-04-30", true},
		{"Jots/2026-04-30.md", "2026-04-30", true},
		{"Notes/Some Note.md", "", false},
		{"2026-4-30.md", "", false}, // strict YYYY-MM-DD
		{"2026-04-30-foo.md", "", false},
	}
	for _, c := range cases {
		got, ok := dailyDate(c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("dailyDate(%q) = %q,%v, want %q,%v", c.path, got, ok, c.want, c.ok)
		}
	}
}
