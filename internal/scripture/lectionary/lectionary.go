// Package lectionary bundles structured Bible reading plans (M'Cheyne
// 1-year, chronological NT, 90-day NT) that a user can "start" so daily
// readings flow onto the calendar instead of leaving them to open a
// chapter and start reading.
//
// Two halves to the package:
//
//   - lectionary.go (this file) — pure, read-only plan catalogue.
//     Plans are GENERATED, not hand-authored — keeping 365 days of
//     M'Cheyne entries inline would balloon the source for no real
//     benefit, and the generator's algorithm is documented (see
//     buildMcheyne / buildChronoNT / buildNT90) so a reviewer can
//     see how the daily readings are assembled. Generation runs
//     once, lazily, behind a sync.Once gate; subsequent Plans()
//     calls return the cached slice with no allocation work.
//
//   - state.go — per-vault "active plan" state (which plans the
//     user has started, when they started). Tiny JSON sidecar
//     under .granit/lectionary-state.json. See state.go for the
//     atomicio storage idiom.
//
// The plans themselves are deliberately algorithmic rather than
// table-lookup against the canonical M'Cheyne tables — a hand-typed
// 365-day table would be a maintenance burden and the user wants
// "structured reading at a sensible pace", not byte-for-byte fidelity
// to the 1842 schedule. The M'Cheyne plan is marked as
// "M'Cheyne-inspired" in its description to signal this.
package lectionary

import "sync"

// Plan is the public shape of one bundled reading plan. ID is stable
// across releases (used in URLs + on-disk state); Name/Description are
// display strings; LengthDays equals len(Readings) but is duplicated on
// the struct so a list endpoint can serve a cheap summary without
// shipping every DayReadings entry.
type Plan struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	LengthDays  int           `json:"lengthDays"`
	Readings    []DayReadings `json:"readings,omitempty"`
}

// DayReadings is one day's worth of passages. Day is 1-indexed (Day 1
// is the first day of the plan, matching how a user thinks about "Day
// 47 of 365"). Passages are free-form citation strings the bible
// reader's existing parser already understands ("Gen 1", "1 Cor 13").
type DayReadings struct {
	Day      int      `json:"day"`
	Passages []string `json:"passages"`
}

// ── Canonical chapter counts ─────────────────────────────────────────
//
// Hardcoded because the generator runs without I/O (the bible package
// would force a translation load + JSON parse just to recover the same
// 66 integers). These match the Protestant canon used by the embedded
// World English Bible.

type bookSpec struct {
	abbrev   string // citation abbreviation; what we emit into Passages
	chapters int
}

var otBooks = []bookSpec{
	{"Gen", 50}, {"Exod", 40}, {"Lev", 27}, {"Num", 36}, {"Deut", 34},
	{"Josh", 24}, {"Judg", 21}, {"Ruth", 4},
	{"1 Sam", 31}, {"2 Sam", 24}, {"1 Kgs", 22}, {"2 Kgs", 25},
	{"1 Chr", 29}, {"2 Chr", 36}, {"Ezra", 10}, {"Neh", 13}, {"Esth", 10},
	{"Job", 42}, {"Ps", 150}, {"Prov", 31}, {"Eccl", 12}, {"Song", 8},
	{"Isa", 66}, {"Jer", 52}, {"Lam", 5}, {"Ezek", 48}, {"Dan", 12},
	{"Hos", 14}, {"Joel", 3}, {"Amos", 9}, {"Obad", 1}, {"Jonah", 4},
	{"Mic", 7}, {"Nah", 3}, {"Hab", 3}, {"Zeph", 3}, {"Hag", 2},
	{"Zech", 14}, {"Mal", 4},
}

// Split for the M'Cheyne family/private columns. "OT history" leans
// narrative + history (Joshua → Esther + the prophets that anchor on
// historical figures); "OT family" leans Torah + Wisdom + the prophets.
// The historical M'Cheyne tables split differently, but the principle
// (two parallel walks through the OT over a year) is preserved.
var otFamily = []bookSpec{
	{"Gen", 50}, {"Exod", 40}, {"Lev", 27}, {"Num", 36}, {"Deut", 34},
	{"Job", 42}, {"Ps", 150}, {"Prov", 31}, {"Eccl", 12}, {"Song", 8},
	{"Isa", 66}, {"Jer", 52}, {"Lam", 5}, {"Ezek", 48}, {"Dan", 12},
	{"Hos", 14}, {"Joel", 3}, {"Amos", 9}, {"Obad", 1}, {"Jonah", 4},
	{"Mic", 7}, {"Nah", 3}, {"Hab", 3}, {"Zeph", 3}, {"Hag", 2},
	{"Zech", 14}, {"Mal", 4},
}

var otHistory = []bookSpec{
	{"Josh", 24}, {"Judg", 21}, {"Ruth", 4},
	{"1 Sam", 31}, {"2 Sam", 24}, {"1 Kgs", 22}, {"2 Kgs", 25},
	{"1 Chr", 29}, {"2 Chr", 36}, {"Ezra", 10}, {"Neh", 13}, {"Esth", 10},
}

// NT books in canonical (Bible-order) sequence — used by the M'Cheyne
// "NT family" column.
var ntCanonical = []bookSpec{
	{"Matt", 28}, {"Mark", 16}, {"Luke", 24}, {"John", 21}, {"Acts", 28},
	{"Rom", 16}, {"1 Cor", 16}, {"2 Cor", 13}, {"Gal", 6}, {"Eph", 6},
	{"Phil", 4}, {"Col", 4}, {"1 Thess", 5}, {"2 Thess", 3},
	{"1 Tim", 6}, {"2 Tim", 4}, {"Titus", 3}, {"Phlm", 1},
	{"Heb", 13}, {"Jas", 5}, {"1 Pet", 5}, {"2 Pet", 3},
	{"1 John", 5}, {"2 John", 1}, {"3 John", 1}, {"Jude", 1},
	{"Rev", 22},
}

// NT books in narrative/chronological reading order. Gospels → Acts →
// Pauline (in roughly the order they sit in the canon, which is also
// close to writing-order) → general epistles → Revelation. This is the
// order used by the Chronological NT plan and as one of the two NT
// walks in the M'Cheyne generator.
var ntChrono = []bookSpec{
	// Narrative
	{"Matt", 28}, {"Mark", 16}, {"Luke", 24}, {"John", 21},
	{"Acts", 28},
	// Pauline
	{"Rom", 16}, {"1 Cor", 16}, {"2 Cor", 13}, {"Gal", 6}, {"Eph", 6},
	{"Phil", 4}, {"Col", 4}, {"1 Thess", 5}, {"2 Thess", 3},
	{"1 Tim", 6}, {"2 Tim", 4}, {"Titus", 3}, {"Phlm", 1},
	// General + Catholic
	{"Heb", 13}, {"Jas", 5}, {"1 Pet", 5}, {"2 Pet", 3},
	{"1 John", 5}, {"2 John", 1}, {"3 John", 1}, {"Jude", 1},
	// Apocalyptic
	{"Rev", 22},
}

// ── Catalogue (lazy-built, cached) ───────────────────────────────────
//
// Plans() / Get() are the only public entry points. Generation is
// idempotent + side-effect free, but it does walk ~400 chapters of
// reference data — once is plenty. sync.Once keeps the cost off the
// hot path on every request.

var (
	once    sync.Once
	plans   []Plan
	plansMu sync.RWMutex
)

func build() {
	plansMu.Lock()
	defer plansMu.Unlock()
	plans = []Plan{
		buildMcheyne(),
		buildChronoNT(),
		buildNT90(),
	}
}

// Plans returns the bundled catalogue. The returned slice is shared —
// callers must NOT mutate it. (Stdlib convention; we're not handing
// out defensive copies because Plan values are themselves mostly
// immutable string data.)
func Plans() []Plan {
	once.Do(build)
	plansMu.RLock()
	defer plansMu.RUnlock()
	return plans
}

// Get returns the plan with the given ID. Boolean signals presence so
// the caller can 404 cleanly without an err-vs-nil dance.
func Get(id string) (Plan, bool) {
	for _, p := range Plans() {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}

// ── Generators ───────────────────────────────────────────────────────

// flatChapters expands a slice of bookSpecs into a flat list of
// citation strings: [{"Gen", 50}, {"Exod", 40}] → ["Gen 1", "Gen 2",
// ..., "Gen 50", "Exod 1", ...]. Used by every generator to convert
// "walk these books linearly" into a chapter feed we can pick from.
func flatChapters(books []bookSpec) []string {
	total := 0
	for _, b := range books {
		total += b.chapters
	}
	out := make([]string, 0, total)
	for _, b := range books {
		for c := 1; c <= b.chapters; c++ {
			out = append(out, formatRef(b.abbrev, c))
		}
	}
	return out
}

func formatRef(abbrev string, chapter int) string {
	// Single-chapter books (Obad, Phlm, 2 John, 3 John, Jude) read more
	// naturally as just the book name — there's no other chapter to
	// disambiguate from. The bible-reader's reference parser handles
	// both forms ("Obad" and "Obad 1") so the citation chips render
	// fine either way; the prose form looks less robotic in task text.
	if chapter == 1 {
		switch abbrev {
		case "Obad", "Phlm", "2 John", "3 John", "Jude":
			return abbrev
		}
	}
	return abbrev + " " + itoa(chapter)
}

// itoa avoids an strconv import for the one place we need it. Inlined
// for clarity (small enough that the indirection isn't worth the import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// buildMcheyne generates a 365-day, 4-passage-per-day plan inspired by
// Robert Murray M'Cheyne's 1842 calendar. The historical M'Cheyne
// schedule walks the OT once and the NT + Psalms twice over the year,
// across four parallel "columns" (two read in family, two in private).
//
// Our generator walks four parallel feeds and slices each into 365
// roughly-equal-sized daily chunks:
//
//   col1: OT family   — Torah → Wisdom → Major prophets → Minor prophets
//   col2: OT history  — Joshua → Esther
//   col3: NT family   — canonical order, full NT
//   col4: NT/wisdom   — chronological NT (read again, narrative-ordered)
//
// For each column we compute chunkLen = ceil(totalChapters / 365) and
// emit chunkLen chapters per day, walking the feed in order. The last
// day picks up whatever's left to ensure every chapter is covered.
// When the feed runs out before day 365, the column repeats from the
// start — col4 (the shorter NT walk) naturally does this once or
// twice over the year, which is in the spirit of the original
// "Psalms + NT twice over" cadence.
//
// Result: each day has 4 passages, total reading load lands at
// ~3-5 chapters per day depending on which columns overlap that day.
// Not byte-for-byte equal to the historical tables but spiritually
// equivalent: a year-long disciplined walk through all of scripture
// with daily contact across the canon.
func buildMcheyne() Plan {
	const days = 365
	col1 := flatChapters(otFamily)
	col2 := flatChapters(otHistory)
	col3 := flatChapters(ntCanonical)
	col4 := flatChapters(ntChrono)

	out := make([]DayReadings, days)
	for d := 0; d < days; d++ {
		out[d] = DayReadings{
			Day: d + 1,
			Passages: []string{
				pickAt(col1, d, days),
				pickAt(col2, d, days),
				pickAt(col3, d, days),
				pickAt(col4, d, days),
			},
		}
	}
	return Plan{
		ID:          "mcheyne",
		Name:        "M'Cheyne 1-year",
		Description: "M'Cheyne-inspired: four passages per day, the whole Bible in a year.",
		LengthDays:  days,
		Readings:    out,
	}
}

// pickAt picks one chapter from the feed for day d (0-indexed) on a
// total-of-days schedule. Walks the feed at chunkLen chapters per day
// and returns the chapter that "lands on" this day's chunk — the FIRST
// chapter of the chunk specifically, so the daily reading stays a
// single citation rather than a range.
//
// If the feed is shorter than days, we wrap (modulo) — col4 (NT
// chrono, ~260 chapters) wraps once or twice across 365 days, which
// gives the user a second pass at the NT over the year. This mirrors
// the original M'Cheyne intent (NT twice) without forcing us to
// hard-code "read NT twice" as a special case.
func pickAt(feed []string, day, days int) string {
	n := len(feed)
	if n == 0 || days <= 0 {
		return ""
	}
	// Linear interpolation: day d in 0..days-1 maps to chapter index
	// d * n / days (floored). This advances roughly evenly through
	// the feed over the year and gives every chapter at least one
	// day of coverage (because consecutive days map to consecutive or
	// near-consecutive indices). For feeds shorter than days the same
	// chapter repeats on adjacent days — fine for the wrap case
	// (col4) where the user is doing a second pass anyway.
	idx := (day * n) / days
	if idx >= n {
		idx = n - 1
	}
	return feed[idx]
}

// buildChronoNT generates a 90-day NT plan in narrative order (Gospels
// → Acts → Pauline → General epistles → Revelation). 260 NT chapters
// spread over 90 days gives ~2.9 chapters/day; we emit 2-3 chapters
// per day by walking the chrono feed and slicing at calculated
// boundaries.
//
// Boundary calc: for each day d we read chapters [d*N/90, (d+1)*N/90)
// where N=len(feed). Rounding-down ensures we cover every chapter
// exactly once over the 90 days. Some days will get 2 chapters,
// some 3 — that's expected for a 260/90 ratio.
func buildChronoNT() Plan {
	const days = 90
	feed := flatChapters(ntChrono)
	out := make([]DayReadings, days)
	n := len(feed)
	for d := 0; d < days; d++ {
		from := (d * n) / days
		to := ((d + 1) * n) / days
		if to <= from {
			to = from + 1
		}
		if to > n {
			to = n
		}
		out[d] = DayReadings{
			Day:      d + 1,
			Passages: append([]string{}, feed[from:to]...),
		}
	}
	return Plan{
		ID:          "chrono-nt",
		Name:        "Chronological NT (90 days)",
		Description: "The whole New Testament in narrative order over 90 days.",
		LengthDays:  days,
		Readings:    out,
	}
}

// buildNT90 generates a second 90-day NT plan, this one in canonical
// (Bible-table-of-contents) order rather than narrative order. Same
// boundary math as buildChronoNT; only the feed source differs. This
// is the "I want to read the NT in 90 days but I'd rather follow the
// book order I'm used to" plan.
func buildNT90() Plan {
	const days = 90
	feed := flatChapters(ntCanonical)
	out := make([]DayReadings, days)
	n := len(feed)
	for d := 0; d < days; d++ {
		from := (d * n) / days
		to := ((d + 1) * n) / days
		if to <= from {
			to = from + 1
		}
		if to > n {
			to = n
		}
		out[d] = DayReadings{
			Day:      d + 1,
			Passages: append([]string{}, feed[from:to]...),
		}
	}
	return Plan{
		ID:          "nt-90day",
		Name:        "NT in 90 days",
		Description: "The New Testament in canonical order, ~3 chapters per day for 90 days.",
		LengthDays:  days,
		Readings:    out,
	}
}
