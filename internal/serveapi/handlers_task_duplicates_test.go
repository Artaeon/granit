package serveapi

import (
	"reflect"
	"testing"
)

func TestNormaliseTaskTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty input", "", nil},
		{"plain prose tokenises lowercase", "Call Mom about birthday",
			[]string{"call", "mom", "about", "birthday"}},
		{"priority marker stripped", "fix login bug !1",
			[]string{"fix", "login", "bug"}},
		{"hashtag stripped", "review #frontend pr",
			[]string{"review", "pr"}},
		{"due marker stripped", "ship feature due:tomorrow",
			[]string{"ship", "feature"}},
		{"estimate marker stripped", "write summary est:30m",
			[]string{"write", "summary"}},
		{"sidecar ref stripped", "talk to alice ^abc123",
			[]string{"talk", "alice"}},
		{"wikilink stripped", "follow up on [[meeting notes]]",
			[]string{"follow"}},
		{"stopwords removed", "the report is on the way",
			[]string{"report", "way"}},
		{"1-char tokens dropped", "a b call mom",
			[]string{"call", "mom"}},
		{"punctuation splits but doesn't emit", "call, mom; ok.",
			[]string{"call", "mom", "ok"}},
		{"multi-space + tabs collapse", "  call\tmom   ",
			[]string{"call", "mom"}},
		{"digits stay as part of token", "ship v2 feature",
			[]string{"ship", "v2", "feature"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normaliseTaskTokens(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("normaliseTaskTokens(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestJaccard(t *testing.T) {
	mk := func(xs ...string) map[string]struct{} {
		out := make(map[string]struct{}, len(xs))
		for _, x := range xs {
			out[x] = struct{}{}
		}
		return out
	}

	t.Run("identical sets → 1", func(t *testing.T) {
		a := mk("call", "mom")
		b := mk("call", "mom")
		if got := jaccard(a, b); got != 1.0 {
			t.Errorf("identical = %v, want 1", got)
		}
	})

	t.Run("disjoint sets → 0", func(t *testing.T) {
		a := mk("call", "mom")
		b := mk("ship", "feature")
		if got := jaccard(a, b); got != 0.0 {
			t.Errorf("disjoint = %v, want 0", got)
		}
	})

	t.Run("partial overlap maths", func(t *testing.T) {
		// {a,b,c} ∩ {b,c,d} = {b,c} (2)
		// {a,b,c} ∪ {b,c,d} = {a,b,c,d} (4)
		// 2/4 = 0.5
		a := mk("a", "b", "c")
		b := mk("b", "c", "d")
		if got := jaccard(a, b); got != 0.5 {
			t.Errorf("partial overlap = %v, want 0.5", got)
		}
	})

	t.Run("subset → ratio of small/large", func(t *testing.T) {
		// {b,c} fully inside {a,b,c,d}
		// inter = 2, union = 4, jaccard = 0.5
		a := mk("b", "c")
		b := mk("a", "b", "c", "d")
		if got := jaccard(a, b); got != 0.5 {
			t.Errorf("subset = %v, want 0.5", got)
		}
	})

	t.Run("empty set → 0 (avoid NaN)", func(t *testing.T) {
		empty := map[string]struct{}{}
		a := mk("call", "mom")
		if got := jaccard(empty, a); got != 0 {
			t.Errorf("empty vs non-empty = %v, want 0", got)
		}
		if got := jaccard(a, empty); got != 0 {
			t.Errorf("non-empty vs empty = %v, want 0", got)
		}
		if got := jaccard(empty, empty); got != 0 {
			t.Errorf("empty vs empty = %v, want 0", got)
		}
	})

	t.Run("symmetric — swap order, same answer", func(t *testing.T) {
		a := mk("call", "mom")
		b := mk("call", "phone", "mom")
		ab := jaccard(a, b)
		ba := jaccard(b, a)
		if ab != ba {
			t.Errorf("asymmetric: jaccard(a,b)=%v but jaccard(b,a)=%v", ab, ba)
		}
	})
}

// Integration-flavoured: end-to-end through normaliseTaskTokens →
// build sets → jaccard. The motivating duplicate case the user
// described: "call mom" vs "phone mom" — same intent, different
// verb. With stopwords removed, "call" + "mom" vs "phone" + "mom"
// → inter={mom}, union={call,phone,mom}, jaccard=1/3 ≈ 0.33. Below
// the 0.6 threshold, so we'd NOT flag those as duplicates — verb
// swap alone isn't enough signal. Worth pinning so we know if
// stopword tuning changes that.
func TestDuplicateDetection_VerbSwap_NotMatched(t *testing.T) {
	a := normaliseTaskTokens("call mom")
	b := normaliseTaskTokens("phone mom")
	setA := tokensToSet(a)
	setB := tokensToSet(b)
	sim := jaccard(setA, setB)
	if sim >= duplicateThreshold {
		t.Errorf("call mom / phone mom similarity = %v, expected < threshold %v", sim, duplicateThreshold)
	}
}

// "fix login bug" vs "fix the login bug !1" — same task, slight
// noise. Stopwords + marker stripping should make these match.
func TestDuplicateDetection_NoiseTolerance_Matched(t *testing.T) {
	a := normaliseTaskTokens("fix login bug")
	b := normaliseTaskTokens("fix the login bug !1")
	setA := tokensToSet(a)
	setB := tokensToSet(b)
	sim := jaccard(setA, setB)
	if sim < duplicateThreshold {
		t.Errorf("noise-only diff similarity = %v, expected >= threshold %v", sim, duplicateThreshold)
	}
}

func tokensToSet(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}
