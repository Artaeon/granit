package tasks

import (
	"sort"
	"time"
)

// reconcile glues parsed markdown lines back to their sidecar
// entries via stable IDs. The 6 passes (in priority order):
//
//   1. exact (file, fingerprint)         — same line, ignore mutable metadata edits
//   2. same-file fingerprint drift        — line moved within the file
//   3. cross-file fingerprint move        — task migrated to a different note
//   4. same-fingerprint disambiguation    — duplicate text resolved by line proximity
//   5. fuzzy norm_text match (gated)      — wording edited, still recognizably the same task
//   6. mint new ULID                      — first-time-seen line
//
// Every parsed line ends up assigned to exactly one ID. Sidecar
// entries with no parsed-line match become orphans: marked the
// first cycle, tombstoned the second cycle. This delay survives
// transient empty-file states (e.g. the editor clearing the file
// just before saving the new content).
type reconcileInput struct {
	parsed  []Task          // freshly parsed from markdown — IDs unset
	sidecar sidecarFile     // last persisted state
	now     time.Time
	newID   func() string   // injected for deterministic tests
}

type reconcileOutput struct {
	tasks   []Task          // parsed tasks with IDs assigned
	sidecar sidecarFile     // updated sidecar (anchors refreshed, tombstones added)
	created []string        // IDs minted this pass
	updated []string        // IDs whose anchor or text changed
	deleted []string        // IDs converted to tombstones this pass
}

// reconcile runs the 6-pass match. Pure function — no I/O, no
// global state. Tests can drive it with synthetic sidecars and
// parsed slices and inspect the output diff.
func reconcile(in reconcileInput) reconcileOutput {
	out := reconcileOutput{
		tasks:   make([]Task, 0, len(in.parsed)),
		sidecar: sidecarFile{Schema: sidecarSchemaVersion},
	}

	// Build sidecar lookup indices.
	sideByFile := make(map[string]map[string][]int) // file → fp → []sidecar idx
	sideByFP := make(map[string][]int)              // fp → []sidecar idx
	for i, st := range in.sidecar.Tasks {
		if sideByFile[st.Anchor.File] == nil {
			sideByFile[st.Anchor.File] = make(map[string][]int)
		}
		sideByFile[st.Anchor.File][st.Fingerprint] = append(sideByFile[st.Anchor.File][st.Fingerprint], i)
		sideByFP[st.Fingerprint] = append(sideByFP[st.Fingerprint], i)
	}

	matchedSide := make(map[int]bool, len(in.sidecar.Tasks)) // sidecar idx → true
	matchedParsed := make(map[int]string, len(in.parsed))    // parsed idx → assigned ID

	// Per-parsed-line precomputed fingerprints — we'll need them in
	// every pass.
	parsedFP := make([]string, len(in.parsed))
	parsedNorm := make([]string, len(in.parsed))
	parsedByFP := make(map[string][]int) // fp → []parsed idx
	for i, t := range in.parsed {
		parsedFP[i] = Fingerprint(t.Text)
		parsedNorm[i] = NormalizeTaskText(t.Text)
		parsedByFP[parsedFP[i]] = append(parsedByFP[parsedFP[i]], i)
	}

	// PASS 1: exact (file, fingerprint) match. Strongest signal —
	// covers done-toggle, indent change, due-date update.
	for i, t := range in.parsed {
		if _, taken := matchedParsed[i]; taken {
			continue
		}
		candidates := sideByFile[t.NotePath][parsedFP[i]]
		picked := -1
		for _, idx := range candidates {
			if matchedSide[idx] {
				continue
			}
			if in.sidecar.Tasks[idx].Anchor.Line == t.LineNum {
				picked = idx
				break
			}
		}
		if picked >= 0 {
			matchedSide[picked] = true
			matchedParsed[i] = in.sidecar.Tasks[picked].ID
		}
	}

	// PASS 2: same-file fingerprint drift. Line moved within the
	// file (vim swap, paragraph reorder).
	for i, t := range in.parsed {
		if _, taken := matchedParsed[i]; taken {
			continue
		}
		candidates := sideByFile[t.NotePath][parsedFP[i]]
		// Pick the unmatched candidate closest to the new line.
		picked, bestDelta := -1, 1<<30
		for _, idx := range candidates {
			if matchedSide[idx] {
				continue
			}
			delta := abs(in.sidecar.Tasks[idx].Anchor.Line - t.LineNum)
			if delta < bestDelta {
				picked, bestDelta = idx, delta
			}
		}
		if picked >= 0 {
			matchedSide[picked] = true
			matchedParsed[i] = in.sidecar.Tasks[picked].ID
		}
	}

	// PASS 3: cross-file fingerprint move. Task cut from one note,
	// pasted into another. Only honors the match if the previous
	// home file no longer contains an unmatched fp candidate.
	for i, t := range in.parsed {
		if _, taken := matchedParsed[i]; taken {
			continue
		}
		candidates := sideByFP[parsedFP[i]]
		// Prefer the candidate whose old file no longer has an
		// unmatched entry with this fp at all.
		picked := -1
		for _, idx := range candidates {
			if matchedSide[idx] {
				continue
			}
			oldFile := in.sidecar.Tasks[idx].Anchor.File
			if oldFile == t.NotePath {
				continue // Pass 2 should have caught this
			}
			// Check whether oldFile still has unmatched fp entries.
			stillThere := false
			for _, otherIdx := range sideByFile[oldFile][parsedFP[i]] {
				if !matchedSide[otherIdx] && otherIdx != idx {
					stillThere = true
					break
				}
			}
			if !stillThere {
				picked = idx
				break
			}
		}
		if picked >= 0 {
			matchedSide[picked] = true
			matchedParsed[i] = in.sidecar.Tasks[picked].ID
		}
	}

	// PASS 4: ambiguous-fp disambiguation. Two parsed lines share
	// a fingerprint with two unmatched sidecar entries — assign by
	// minimal line-distance to the previous anchor.
	for i, t := range in.parsed {
		if _, taken := matchedParsed[i]; taken {
			continue
		}
		candidates := sideByFP[parsedFP[i]]
		picked, bestDelta := -1, 1<<30
		for _, idx := range candidates {
			if matchedSide[idx] {
				continue
			}
			anchor := in.sidecar.Tasks[idx].Anchor
			delta := abs(anchor.Line-t.LineNum) + fileDistance(anchor.File, t.NotePath)
			if delta < bestDelta {
				picked, bestDelta = idx, delta
			}
		}
		if picked >= 0 {
			matchedSide[picked] = true
			matchedParsed[i] = in.sidecar.Tasks[picked].ID
		}
	}

	// PASS 5: fuzzy norm_text match. Last resort, gated. Only fires
	// for sidecar entries when no PARSED line still carries this
	// sidecar's fingerprint — that absence is what tells us the user
	// edited the wording rather than just moving the line.
	// Damerau-Levenshtein ratio ≥ 0.85 and line delta ≤ 10 are both
	// required to attribute the match.
	for sideIdx, st := range in.sidecar.Tasks {
		if matchedSide[sideIdx] {
			continue
		}
		stillInParsed := false
		for _, pIdx := range parsedByFP[st.Fingerprint] {
			if _, taken := matchedParsed[pIdx]; !taken {
				stillInParsed = true
				break
			}
		}
		if stillInParsed {
			continue
		}
		picked, bestRatio := -1, 0.0
		for parsedIdx, t := range in.parsed {
			if _, taken := matchedParsed[parsedIdx]; taken {
				continue
			}
			if t.NotePath != st.Anchor.File {
				continue
			}
			if abs(t.LineNum-st.Anchor.Line) > 10 {
				continue
			}
			ratio := similarityRatio(parsedNorm[parsedIdx], st.NormText)
			if ratio >= 0.85 && ratio > bestRatio {
				picked, bestRatio = parsedIdx, ratio
			}
		}
		if picked >= 0 {
			matchedSide[sideIdx] = true
			matchedParsed[picked] = st.ID
		}
	}

	// PASS 6: mint new IDs for unmatched parsed lines.
	for i := range in.parsed {
		if _, taken := matchedParsed[i]; !taken {
			id := in.newID()
			matchedParsed[i] = id
			out.created = append(out.created, id)
		}
	}

	// Build the output: parsed Tasks with assigned IDs + carried-over
	// sidecar metadata, plus a fresh sidecar.
	for i, t := range in.parsed {
		id := matchedParsed[i]
		t.ID = id

		var st *sidecarTask
		// Find the sidecar entry that was matched to this ID, if any.
		for j := range in.sidecar.Tasks {
			if in.sidecar.Tasks[j].ID == id {
				st = &in.sidecar.Tasks[j]
				break
			}
		}
		// Or revive from a tombstone (re-introduced via git pull).
		var revivedFromTomb bool
		if st == nil {
			for ti, tomb := range in.sidecar.Tombstones {
				if tomb.Fingerprint == parsedFP[i] {
					id = tomb.ID
					t.ID = id
					revivedFromTomb = true
					// Drop this tombstone — the task is back.
					in.sidecar.Tombstones = append(in.sidecar.Tombstones[:ti], in.sidecar.Tombstones[ti+1:]...)
					break
				}
			}
		}

		if st != nil {
			t.Triage = st.Triage
			t.ScheduledStart = st.ScheduledStart
			t.Duration = time.Duration(st.DurationMinutes) * time.Minute
			t.ProjectID = st.ProjectID
			if t.GoalID == "" {
				t.GoalID = st.GoalID
			}
			t.Origin = st.Origin
			t.CreatedAt = st.CreatedAt
			t.LastTriagedAt = st.LastTriagedAt
			t.CompletedAt = st.CompletedAt
			t.Notes = st.Notes
			// Mark drift if the anchor moved.
			if st.Anchor.File != t.NotePath || st.Anchor.Line != t.LineNum {
				out.updated = append(out.updated, id)
			}
		} else {
			// Brand-new (or revived) entry — set defaults.
			if t.Triage == "" {
				t.Triage = TriageInbox
			}
			if t.Origin == "" {
				if isDailyNoteFile(t.NotePath) {
					t.Origin = OriginJot
				} else {
					t.Origin = OriginManual
				}
			}
			if t.CreatedAt.IsZero() {
				t.CreatedAt = in.now
			}
			if revivedFromTomb {
				out.updated = append(out.updated, id)
			}
		}
		// If task is done, ensure CompletedAt is set.
		if t.Done && t.CompletedAt == nil {
			now := in.now
			t.CompletedAt = &now
		}
		if !t.Done {
			t.CompletedAt = nil
		}

		out.tasks = append(out.tasks, t)
		out.sidecar.Tasks = append(out.sidecar.Tasks, sidecarTask{
			ID:              t.ID,
			Fingerprint:     parsedFP[i],
			Anchor:          sidecarAnchor{File: t.NotePath, Line: t.LineNum, Indent: t.Indent},
			NormText:        parsedNorm[i],
			Triage:          t.Triage,
			ScheduledStart:  t.ScheduledStart,
			DurationMinutes: int(t.Duration / time.Minute),
			ProjectID:       t.ProjectID,
			GoalID:          t.GoalID,
			Origin:          t.Origin,
			CreatedAt:       t.CreatedAt,
			LastTriagedAt:   t.LastTriagedAt,
			CompletedAt:     t.CompletedAt,
			Notes:           t.Notes,
		})
	}

	// Carry over still-active tombstones.
	out.sidecar.Tombstones = append(out.sidecar.Tombstones, in.sidecar.Tombstones...)

	// Tombstone unmatched sidecar entries (they were deleted).
	for sideIdx, st := range in.sidecar.Tasks {
		if matchedSide[sideIdx] {
			continue
		}
		out.sidecar.Tombstones = append(out.sidecar.Tombstones, sidecarTombstone{
			ID:          st.ID,
			Fingerprint: st.Fingerprint,
			NormText:    st.NormText,
			RemovedAt:   in.now,
		})
		out.deleted = append(out.deleted, st.ID)
	}

	// Sort tasks by ID for stable iteration.
	sort.Slice(out.tasks, func(i, j int) bool { return out.tasks[i].ID < out.tasks[j].ID })

	return out
}

// abs is the int abs helper Go's stdlib still doesn't expose.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// fileDistance is 0 for same-file, large constant for different
// file. Used by Pass 4 to bias disambiguation toward same-file
// candidates.
func fileDistance(a, b string) int {
	if a == b {
		return 0
	}
	return 10000
}

// similarityRatio is a Damerau-Levenshtein-based ratio in [0, 1].
// 1.0 = identical, 0.0 = totally different. Used by Pass 5 to gate
// fuzzy matches at ≥ 0.85.
func similarityRatio(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	dist := levenshtein(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// levenshtein computes the standard Levenshtein edit distance.
// O(len(a) × len(b)) time and space. Only called for fuzzy fallback
// (Pass 5) on short normalized task strings — trivial cost.
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = minInt(
				prev[j]+1,      // deletion
				cur[j-1]+1,     // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func minInt(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// isDailyNoteFile recognizes daily-note paths (YYYY-MM-DD.md
// anywhere in the path) so first-ingestion can default Origin to
// OriginJot for tasks captured in jots. Conservative match — must
// be a basename like 2026-04-25.md.
func isDailyNoteFile(path string) bool {
	if len(path) < 13 { // "YYYY-MM-DD.md"
		return false
	}
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			base = path[i+1:]
			break
		}
	}
	if len(base) != 13 || base[10:] != ".md" {
		return false
	}
	for i, ch := range base[:10] {
		if i == 4 || i == 7 {
			if ch != '-' {
				return false
			}
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
