package serveapi

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

type statEntry struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type vaultStats struct {
	NoteCount       int          `json:"noteCount"`
	TotalWords      int          `json:"totalWords"`
	TotalLinks      int          `json:"totalLinks"`
	TotalTags       int          `json:"totalTags"`
	UntypedNotes    int          `json:"untypedNotes"`
	OrphanNotes     int          `json:"orphanNotes"`
	AverageWords    int          `json:"averageWords"`
	NotesPerMonth   []statEntry  `json:"notesPerMonth"`
	TopTags         []statEntry  `json:"topTags"`
	TopLinkedNotes  []statEntry  `json:"topLinkedNotes"`
	LargestNotes    []statEntry  `json:"largestNotes"`
	RecentlyEdited  []statEntry  `json:"recentlyEdited"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	notes := s.cfg.Vault.SnapshotNotes()
	out := vaultStats{
		NoteCount:      len(notes),
		NotesPerMonth:  []statEntry{},
		TopTags:        []statEntry{},
		TopLinkedNotes: []statEntry{},
		LargestNotes:   []statEntry{},
		RecentlyEdited: []statEntry{},
	}

	tagCounts := map[string]int{}
	monthCounts := map[string]int{}
	noteWords := map[string]int{}      // path → words
	backlinkCounts := map[string]int{} // path → incoming-link count
	titleToPath := map[string]string{}
	for _, n := range notes {
		titleToPath[strings.ToLower(n.Title)] = n.RelPath
	}

	for _, n := range notes {
		s.cfg.Vault.EnsureLoaded(n.RelPath)
		// Words
		w := wordCount(n.Content)
		noteWords[n.RelPath] = w
		out.TotalWords += w

		// Tags
		for _, t := range tagsFor(n) {
			tagCounts[t]++
		}

		// Links
		out.TotalLinks += len(n.Links)
		for _, link := range n.Links {
			if p, ok := titleToPath[strings.ToLower(link)]; ok {
				backlinkCounts[p]++
			}
		}

		// Untyped
		if t, _ := n.Frontmatter["type"].(string); t == "" {
			out.UntypedNotes++
		}

		// Notes per month (last 12 months)
		key := n.ModTime.Format("2006-01")
		monthCounts[key]++
	}

	out.TotalTags = len(tagCounts)
	if out.NoteCount > 0 {
		out.AverageWords = out.TotalWords / out.NoteCount
	}

	// Orphan = note with no incoming AND no outgoing links
	for _, n := range notes {
		if len(n.Links) == 0 && backlinkCounts[n.RelPath] == 0 {
			out.OrphanNotes++
		}
	}

	out.TopTags = topN(tagCounts, 10)
	out.TopLinkedNotes = topN(backlinkCounts, 10)
	out.LargestNotes = topN(noteWords, 10)

	// Notes per month: last 12 months in chronological order
	now := time.Now()
	for i := 11; i >= 0; i-- {
		t := now.AddDate(0, -i, 0)
		key := t.Format("2006-01")
		out.NotesPerMonth = append(out.NotesPerMonth, statEntry{Name: key, Value: monthCounts[key]})
	}

	// Recently edited: last 7 (path → timestamp).
	type pm struct {
		path string
		mod  time.Time
	}
	pms := make([]pm, 0, len(notes))
	for _, n := range notes {
		pms = append(pms, pm{path: n.RelPath, mod: n.ModTime})
	}
	sort.Slice(pms, func(i, j int) bool { return pms[i].mod.After(pms[j].mod) })
	for i := 0; i < 7 && i < len(pms); i++ {
		out.RecentlyEdited = append(out.RecentlyEdited, statEntry{Name: pms[i].path, Value: int(pms[i].mod.Unix())})
	}

	writeJSON(w, http.StatusOK, out)
}

func wordCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Fields(s))
}

func topN(m map[string]int, n int) []statEntry {
	out := make([]statEntry, 0, len(m))
	for k, v := range m {
		out = append(out, statEntry{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
