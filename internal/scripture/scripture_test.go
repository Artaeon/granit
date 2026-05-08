package scripture

import (
	"strings"
	"testing"
)

func TestDefaults_NotEmpty(t *testing.T) {
	all := Defaults()
	if len(all) < 50 {
		t.Fatalf("expected at least 50 default verses, got %d", len(all))
	}
	for i, s := range all {
		if strings.TrimSpace(s.Text) == "" {
			t.Errorf("default %d has empty text", i)
		}
		if strings.TrimSpace(s.Source) == "" {
			t.Errorf("default %d (%q) has empty source", i, s.Text)
		}
	}
}

func TestDefaults_TopicsCoverage(t *testing.T) {
	// Most defaults should carry topics so the topical browser has
	// material to work with on a fresh vault.
	all := Defaults()
	tagged := 0
	for _, s := range all {
		if len(s.Topics) > 0 {
			tagged++
		}
	}
	if tagged < len(all)*9/10 {
		t.Errorf("expected ≥90%% of defaults to be tagged with topics, got %d/%d", tagged, len(all))
	}
}

func TestTopics_DerivedFromDefaults(t *testing.T) {
	// Empty path → Load returns Defaults; Topics aggregates from there.
	tcs := Topics("/nonexistent")
	if len(tcs) < 10 {
		t.Fatalf("expected a healthy spread of topics, got %d", len(tcs))
	}
	// Sorted by count desc, then name asc.
	for i := 1; i < len(tcs); i++ {
		prev, cur := tcs[i-1], tcs[i]
		if cur.Count > prev.Count {
			t.Fatalf("topics not sorted by count desc: %+v then %+v", prev, cur)
		}
		if cur.Count == prev.Count && cur.Topic < prev.Topic {
			t.Fatalf("topics with equal count not sorted alphabetically: %+v then %+v", prev, cur)
		}
	}
	// "love" is one of the most-tagged themes — it should appear.
	found := false
	for _, tc := range tcs {
		if tc.Topic == "love" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'love' to be among the default topics")
	}
}

func TestByTopic_FiltersCaseInsensitively(t *testing.T) {
	hits := ByTopic("/nonexistent", "love")
	if len(hits) == 0 {
		t.Fatal("expected at least one verse tagged 'love'")
	}
	for _, s := range hits {
		ok := false
		for _, t := range s.Topics {
			if strings.EqualFold(t, "love") {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("verse %q tagged %v lacked 'love'", s.Text, s.Topics)
		}
	}
	// Mixed case should match identically.
	upper := ByTopic("/nonexistent", "LOVE")
	if len(upper) != len(hits) {
		t.Errorf("case-insensitive lookup mismatch: %d vs %d", len(upper), len(hits))
	}
	// Empty topic returns nil.
	if ByTopic("/nonexistent", "  ") != nil {
		t.Error("expected nil for blank topic")
	}
}
