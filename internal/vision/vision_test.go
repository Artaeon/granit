package vision

import (
	"reflect"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := Vision{
		Mission:         "Build tools that help people live focused lives.",
		Values:          []string{"Faith", "Family", "Craft", "Honesty"},
		SeasonFocus:     "Ship granit web v1",
		SeasonStartedAt: "2026-04-01",
		Notes:           "Quarter started after the spring break review.",
	}
	if err := Save(dir, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := Load(dir)
	// UpdatedAt is stamped on save, so compare every other field
	// individually. UpdatedAt itself just needs to be non-zero.
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be stamped on save")
	}
	in.UpdatedAt = got.UpdatedAt
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip mismatch:\n got=%+v\n want=%+v", got, in)
	}
}

// LoadFromMissingFile returns zero Vision, never panics. The page's
// empty-state path depends on this contract — anything else (nil
// pointer, error) would surface as a console error in the web UI on
// fresh vaults.
func TestLoadFromMissingFile(t *testing.T) {
	dir := t.TempDir()
	got := Load(dir)
	if !got.IsEmpty() {
		t.Errorf("Load on empty dir: got %+v, want zero", got)
	}
	if got.IsEmpty() != true {
		t.Error("IsEmpty should be true on zero Vision")
	}
}

func TestIsEmpty(t *testing.T) {
	cases := []struct {
		name string
		v    Vision
		want bool
	}{
		{"all empty", Vision{}, true},
		{"only notes", Vision{Notes: "stray thought"}, true}, // notes alone don't count
		{"mission set", Vision{Mission: "X"}, false},
		{"values set", Vision{Values: []string{"Faith"}}, false},
		{"season set", Vision{SeasonFocus: "Q1"}, false},
		{"empty values slice", Vision{Values: []string{}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.IsEmpty(); got != c.want {
				t.Errorf("IsEmpty = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSeasonDayCount(t *testing.T) {
	today := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		started   string
		wantDay   int
		wantTotal int
	}{
		{"unset → zeros", "", 0, 0},
		{"day 1 (started today)", "2026-05-01", 1, 90},
		{"day 30 (started Apr 2)", "2026-04-02", 30, 90},
		{"day 91 clamped to 90", "2026-01-15", 90, 90},
		{"future-dated → clamps to day 1", "2026-06-15", 1, 90},
		{"unparseable → zeros", "garbage", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Vision{SeasonStartedAt: c.started}
			d, total := v.SeasonDayCount(today)
			if d != c.wantDay || total != c.wantTotal {
				t.Errorf("SeasonDayCount = (%d, %d), want (%d, %d)", d, total, c.wantDay, c.wantTotal)
			}
		})
	}
}
