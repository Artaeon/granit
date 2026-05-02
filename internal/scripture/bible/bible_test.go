package bible

import (
	"math/rand"
	"strings"
	"testing"
)

// TestLoad smoke-checks that the embedded JSON parses and contains the
// 66-book Protestant canon with expected high-water-mark counts.
func TestLoad(t *testing.T) {
	b, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(b.Books), 66; got != want {
		t.Fatalf("books: got %d want %d", got, want)
	}
	if b.Abbreviation != "WEB" {
		t.Errorf("abbreviation: got %q want %q", b.Abbreviation, "WEB")
	}

	// Spot-check a few canonical anchor verses so a corrupted JSON
	// bundle would be loud rather than silent.
	cases := []struct {
		book, prefix string
		ch, v        int
	}{
		{"Genesis", "In the beginning", 1, 1},
		{"John", "For God so loved the world", 3, 16},
		{"Psalms", "Yahweh is my shepherd", 23, 1},
	}
	for _, tc := range cases {
		bk := FindBook(tc.book)
		if bk == nil {
			t.Errorf("FindBook(%q) returned nil", tc.book)
			continue
		}
		ch := bk.GetChapter(tc.ch)
		if ch == nil {
			t.Errorf("%s ch %d not found", tc.book, tc.ch)
			continue
		}
		var got *Verse
		for i := range ch.Verses {
			if ch.Verses[i].N == tc.v {
				got = &ch.Verses[i]
				break
			}
		}
		if got == nil {
			t.Errorf("%s %d:%d not found", tc.book, tc.ch, tc.v)
			continue
		}
		if !strings.HasPrefix(got.Text, tc.prefix) {
			t.Errorf("%s %d:%d: got %q want prefix %q", tc.book, tc.ch, tc.v, got.Text, tc.prefix)
		}
	}
}

// TestBooks ensures the summary list mirrors Load and reports correct
// chapter counts (these are stable / well-known numbers).
func TestBooks(t *testing.T) {
	books, err := Books()
	if err != nil {
		t.Fatalf("Books: %v", err)
	}
	if got, want := len(books), 66; got != want {
		t.Fatalf("books: got %d want %d", got, want)
	}
	want := map[string]int{
		"Genesis":     50,
		"Psalms":      150,
		"Proverbs":    31,
		"Matthew":     28,
		"John":        21,
		"Revelation":  22,
		"Obadiah":     1,
		"Philemon":    1,
		"3 John":      1,
		"1 Chronicles": 29,
	}
	got := map[string]int{}
	for _, b := range books {
		got[b.Name] = b.Chapters
	}
	for name, c := range want {
		if got[name] != c {
			t.Errorf("%s chapters: got %d want %d", name, got[name], c)
		}
	}
}

// TestFindBookAliases covers the case-folding + alias resolution we
// expose to the API: callers can pass "JHN", "John", or "john" and
// they all hit the same book.
func TestFindBookAliases(t *testing.T) {
	for _, q := range []string{"JHN", "John", "john", "JoHn"} {
		b := FindBook(q)
		if b == nil || b.Code != "JHN" {
			t.Errorf("FindBook(%q) → %v", q, b)
		}
	}
	for _, q := range []string{"1corinthians", "1 Corinthians", "1CO", "1Corinthians"} {
		b := FindBook(q)
		if b == nil || b.Code != "1CO" {
			t.Errorf("FindBook(%q) → %v", q, b)
		}
	}
	if FindBook("Psalm") == nil {
		t.Error("FindBook(Psalm) should resolve via the Psalms alias")
	}
	if FindBook("Songofsongs") == nil {
		t.Error("FindBook(Songofsongs) should resolve to Song of Solomon")
	}
	if FindBook("not-a-book") != nil {
		t.Error("FindBook(not-a-book) should return nil")
	}
}

// TestRandomReturnsValidPassage seeds the RNG so the test is deterministic
// and verifies the result references real verses + a sensible reference.
func TestRandomReturnsValidPassage(t *testing.T) {
	for i := 0; i < 25; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		p, err := Random(RandomOptions{Length: 4, RNG: rng})
		if err != nil {
			t.Fatalf("Random: %v", err)
		}
		if p == nil || len(p.Verses) == 0 {
			t.Fatalf("Random returned empty passage")
		}
		if len(p.Verses) > 4 {
			t.Errorf("passage too long: got %d verses, want <=4", len(p.Verses))
		}
		// Reference matches the verses.
		if p.StartV != p.Verses[0].N || p.EndV != p.Verses[len(p.Verses)-1].N {
			t.Errorf("reference %s doesn't match verses [%d..%d]", p.Reference, p.Verses[0].N, p.Verses[len(p.Verses)-1].N)
		}
		if !strings.Contains(p.Reference, p.Book) {
			t.Errorf("reference %q missing book %q", p.Reference, p.Book)
		}
		// Confirm the verse actually exists in the loaded data.
		bk := FindBook(p.BookCode)
		if bk == nil {
			t.Fatalf("BookCode %q not found", p.BookCode)
		}
		ch := bk.GetChapter(p.Chapter)
		if ch == nil {
			t.Fatalf("%s chapter %d not found", p.Book, p.Chapter)
		}
		var matched int
		for _, want := range p.Verses {
			for _, have := range ch.Verses {
				if have.N == want.N && have.Text == want.Text {
					matched++
					break
				}
			}
		}
		if matched != len(p.Verses) {
			t.Errorf("only %d/%d verses round-trip in source data", matched, len(p.Verses))
		}
	}
}

// TestRandomLengthClamps the Length field gets pinned to [1, 10].
func TestRandomLengthClamps(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	p, err := Random(RandomOptions{Length: 100, RNG: rng})
	if err != nil {
		t.Fatalf("Random: %v", err)
	}
	if len(p.Verses) > 10 {
		t.Errorf("length not clamped: got %d", len(p.Verses))
	}
	rng = rand.New(rand.NewSource(42))
	p, err = Random(RandomOptions{Length: 0, RNG: rng})
	if err != nil {
		t.Fatalf("Random: %v", err)
	}
	if len(p.Verses) == 0 || len(p.Verses) > 4 {
		t.Errorf("default length not 4: got %d", len(p.Verses))
	}
}

// TestRandomBookFilter restricts the candidate pool.
func TestRandomBookFilter(t *testing.T) {
	for i := 0; i < 5; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		p, err := Random(RandomOptions{Length: 2, Book: "Proverbs", RNG: rng})
		if err != nil {
			t.Fatalf("Random: %v", err)
		}
		if p.Book != "Proverbs" {
			t.Errorf("filter ignored: got book %q", p.Book)
		}
	}
}

// TestRandomTestamentFilter restricts to OT/NT.
func TestRandomTestamentFilter(t *testing.T) {
	for i := 0; i < 10; i++ {
		rng := rand.New(rand.NewSource(int64(100 + i)))
		p, err := Random(RandomOptions{Length: 2, Testament: "NT", RNG: rng})
		if err != nil {
			t.Fatalf("Random: %v", err)
		}
		bk := FindBook(p.BookCode)
		if bk == nil || bk.Testament != "NT" {
			t.Errorf("NT filter ignored: got book %q (%s)", p.Book, bk.Testament)
		}
	}
}

// TestSearch spot-checks that an obvious phrase returns hits in
// canonical order and the limit is respected.
func TestSearch(t *testing.T) {
	hits, err := Search("In the beginning", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Search returned no hits for 'In the beginning'")
	}
	// Genesis 1:1 should be the first hit (canonical order).
	if hits[0].BookCode != "GEN" || hits[0].Chapter != 1 || hits[0].Verse != 1 {
		t.Errorf("first hit: got %s, want Genesis 1:1", hits[0].Reference)
	}
	if len(hits) > 5 {
		t.Errorf("limit ignored: got %d hits", len(hits))
	}

	// Empty query → no error, no hits.
	hits, err = Search("   ", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("empty query returned %d hits", len(hits))
	}

	// Case-insensitive: "JESUS" should hit just like "jesus".
	upper, _ := Search("JESUS", 3)
	lower, _ := Search("jesus", 3)
	if len(upper) != len(lower) || (len(upper) > 0 && upper[0].Reference != lower[0].Reference) {
		t.Errorf("case sensitivity broke search: upper=%d lower=%d", len(upper), len(lower))
	}
}
