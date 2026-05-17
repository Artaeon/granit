package serveapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/artaeon/granit/internal/tasks"
)

// Task-duplicate detector. Walks open tasks and pairs any two whose
// normalised token sets overlap above a Jaccard-similarity
// threshold. Returns pairs (not transitive clusters) so the UI's
// "keep this, drop that" gesture stays one-vs-one — clusters need
// an "arbitrator" task and the UX gets complicated fast.
//
// Deterministic (no AI), so it's cheap to run on every request and
// safe to cache nothing. Open tasks only — done / dropped tasks
// shouldn't surface as duplicates of live work. Bounded to TOP-200
// open tasks for an O(n²) scan that stays well under 50ms on a
// modern laptop even at the cap.

type taskDuplicatePair struct {
	A          *tasks.Task `json:"a"`
	B          *tasks.Task `json:"b"`
	Similarity float64     `json:"similarity"` // 0..1, Jaccard token overlap
}

const (
	// Below this threshold two tasks aren't "duplicate enough" to
	// flag. 0.6 means 60%+ of the unioned tokens overlap; a verb
	// swap ("call mom" vs "phone mom") still scores high while
	// merely-related work ("ship feature A" / "ship feature B")
	// stays separated.
	duplicateThreshold = 0.6
	// Hard cap on the candidate set. Pair scan is O(n²) — 200 → 20k
	// comparisons, each a tiny set intersection. Fine.
	duplicateScanCap = 200
	// Hard cap on returned pairs. UI renders one card per pair;
	// past ~25 the user stops scrolling. Pairs return sorted by
	// similarity desc, so the cap drops the lowest matches.
	duplicateResultCap = 25
)

func (s *Server) handleTaskDuplicates(w http.ResponseWriter, r *http.Request) {
	// Optional override for the threshold via ?threshold=0.55 etc.
	// Bounds [0.1, 0.95] so the user can't accidentally fetch
	// everything (0.0) or get an empty result by demanding 1.0.
	threshold := duplicateThreshold
	if v := r.URL.Query().Get("threshold"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0.1 && f <= 0.95 {
			threshold = f
		}
	}

	all := s.cfg.TaskStore.All()
	// Filter to open tasks. The dup-find is about "am I tracking
	// the same thing twice in my live list" — completed / dropped
	// / archived items shouldn't pollute the comparison. Archived
	// is the soft-delete flag from the task drawer's archive
	// gesture; without this filter, an archived "call Mom" would
	// score against a live "phone Mom" and surface as a
	// false-positive pair the user can't even merge into.
	open := make([]*tasks.Task, 0, len(all))
	for _, t := range all {
		if t.Done {
			continue
		}
		if t.Archived {
			continue
		}
		if t.Triage == tasks.TriageDropped {
			continue
		}
		open = append(open, t)
	}
	// Sort newest-first by CreatedAt so the scan cap drops the
	// oldest dust when there are too many open tasks. The
	// "duplicate I just created" case is the high-signal one anyway.
	// Task doesn't expose an UpdatedAt; CreatedAt is the closest
	// stable proxy we have without dipping into history.
	sort.Slice(open, func(i, j int) bool {
		return open[i].CreatedAt.After(open[j].CreatedAt)
	})
	if len(open) > duplicateScanCap {
		open = open[:duplicateScanCap]
	}

	// Pre-tokenise once per task — saves O(n) work per pair.
	tokens := make([][]string, len(open))
	tokenSets := make([]map[string]struct{}, len(open))
	for i, t := range open {
		tk := normaliseTaskTokens(t.Text)
		tokens[i] = tk
		set := make(map[string]struct{}, len(tk))
		for _, w := range tk {
			set[w] = struct{}{}
		}
		tokenSets[i] = set
	}

	pairs := make([]taskDuplicatePair, 0)
	for i := 0; i < len(open); i++ {
		if len(tokenSets[i]) == 0 {
			continue
		}
		for j := i + 1; j < len(open); j++ {
			if len(tokenSets[j]) == 0 {
				continue
			}
			sim := jaccard(tokenSets[i], tokenSets[j])
			if sim < threshold {
				continue
			}
			pairs = append(pairs, taskDuplicatePair{
				A:          open[i],
				B:          open[j],
				Similarity: sim,
			})
		}
	}
	// Highest similarity first; cap to the result limit. Stable
	// sort isn't required — same similarity → either order is fine.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Similarity > pairs[j].Similarity })
	if len(pairs) > duplicateResultCap {
		pairs = pairs[:duplicateResultCap]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pairs":     pairs,
		"threshold": threshold,
		"scanned":   len(open),
	})
}

// normaliseTaskTokens lowercases the task text, strips Markdown
// task-line markers (`!1`, `due:tomorrow`, `#tag`, `est:30m`, `^id`
// sidecar refs), keeps only alphabetical word characters, and
// splits on whitespace. Stopwords are dropped so they don't inflate
// Jaccard scores on otherwise-different tasks ("the report" vs
// "the meeting" — 50% overlap on "the" alone).
//
// Returns the order-preserved token slice; the caller builds the
// set when needed. Deterministic + pure, easy to unit-test.
func normaliseTaskTokens(text string) []string {
	if text == "" {
		return nil
	}
	// Strip the markers granit's quick-add parser uses. Each gets
	// space-replaced so the surrounding words don't merge.
	cleaned := taskMarkerRe.ReplaceAllString(text, " ")
	cleaned = strings.ToLower(cleaned)

	out := make([]string, 0, 8)
	var sb strings.Builder
	flush := func() {
		if sb.Len() == 0 {
			return
		}
		w := sb.String()
		sb.Reset()
		if len(w) < 2 {
			// 1-char tokens carry no signal.
			return
		}
		if _, stop := taskStopwords[w]; stop {
			return
		}
		out = append(out, w)
	}
	for _, r := range cleaned {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// Marker regex covers `!N` priority, `#tag`, `due:YYYY-MM-DD` /
// `due:tomorrow` etc., `est:30m`, `^id` sidecar back-refs, `[[link]]`
// wikilinks. Order-agnostic — the regex engine picks the right
// alternation per occurrence.
var taskMarkerRe = regexp.MustCompile(`(?i)(?:!\d+|#[\w-]+|due:[\w-]+|est:[\w-]+|\^[\w-]+|\[\[[^\]]+\]\])`)

// Stopwords list trimmed to the words that show up in *task* prose
// (not general English) — articles, copulas, common verbs that
// dominate Jaccard scores. Conservative: real signal verbs ("call",
// "write", "review") stay so a verb-noun pair still scores
// distinctly.
var taskStopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "of": {}, "to": {}, "for": {}, "and": {},
	"or": {}, "is": {}, "in": {}, "on": {}, "at": {}, "by": {}, "with": {},
	"that": {}, "this": {}, "it": {}, "be": {}, "my": {}, "do": {}, "i": {},
	"any": {}, "all": {}, "as": {}, "if": {}, "so": {},
}

// jaccard computes |a ∩ b| / |a ∪ b| on two token sets. Returns 0
// when either set is empty (avoids 0/0 NaN); 1 when sets are
// identical.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Iterate the smaller set; the larger one provides O(1) lookup.
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	inter := 0
	for k := range small {
		if _, ok := large[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
