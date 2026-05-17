package textutil

import "testing"

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"zero cap", "hello", 0, ""},
		{"negative cap", "hello", -1, ""},
		{"under cap", "hi", 10, "hi"},
		{"exact cap", "hello", 5, "hello"},
		{"over cap ASCII", "helloworld", 5, "hello…"},
		// The whole point of the package: byte-slicing here would
		// land mid-codepoint and corrupt the "ü". Codepoint-aware
		// slicing keeps the rune whole.
		{"over cap latin extended", "über alles", 4, "über…"},
		{"over cap CJK", "你好世界朋友", 3, "你好世…"},
		// Cap inside the surrogate pair of a 4-byte rune. The byte
		// version would emit a 5-byte invalid sequence; ours keeps
		// the rune whole and stops before it.
		{"over cap emoji", "hi 👋 friend", 4, "hi 👋…"},
		// The bug the package was created to fix: the previous
		// inline code `s[:280] + "…"` on a German note. Verify
		// the rune-aware version doesn't split "ä" mid-codepoint.
		{"German near boundary", "Stratägie", 4, "Stra…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateRunes(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("TruncateRunes(%q, %d) = %q; want %q", tc.in, tc.max, got, tc.want)
			}
		})
	}
}
